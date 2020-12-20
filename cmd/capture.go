/*
Copyright © 2020 NAME HERE <EMAIL ADDRESS>

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/
package cmd

import (
	"bufio"
	"errors"
	"fmt"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/s3/s3manager"
	"github.com/ilyakaznacheev/cleanenv"
	"github.com/spf13/cobra"
	pb "gopkg.in/cheggaaa/pb.v1"
	"io"
	"io/ioutil"
	"github.com/rs/zerolog/log"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"sync/atomic"
	"time"
)

var conf Configuration
var AwsPrefix string


type Configuration struct {
	TempBackupDir      string
	DatabaseUrl        string `env:"DATABASE_URL" env-required:"true" env-description:"Source database url to pull backup from (e.g. postgres://root:password@127.0.0.1:5432/devdb)"`
	SlackWebHook       string `env:"SLACK_WEBHOOK" env-description:"Slack webhook to report informations"`
	AwsBucket          string `env:"AWS_BUCKET" env-required:"true" env-description:"Aws bucket to store dump files"`
	AwsRegion          string `env:"AWS_REGION" env-required:"true" env-description:"Aws bucket region"`
	AwsAccessKeyId     string `env:"AWS_ACCESS_KEY_ID" env-required:"true" env-description:"Aws access key id"`
	AwsSecretAccessKey string `env:"AWS_SECRET_ACCESS_KEY" env-required:"true" env-description:"Aws secret access key"`
	AwsPrefix          string `env:"AWS_PREFIX" env-default:"backup" env-description:"Prefix on your bucket where to store backup"`
}

// captureCmd represents the capture command
var captureCmd = &cobra.Command{
	Use:   "capture",
	Short: "Capture a database dump and place it on s3",
	Long:  ``,
	Run: func(cmd *cobra.Command, args []string) {
		// todo : set log to Europe/Paris DST time (juste like snapshot)
		// todo : log error log to slack
		// todo : version vendors

		err := cleanenv.ReadEnv(&conf)
		if err != nil {
			log.Err(err).Msg("error reading env")
		}
		progress, err := cmd.PersistentFlags().GetBool("progress")
		if err != nil {
			log.Err(err).Msg("error getting progress flag")
		}

		// If flag is set it overrides conf file or env
		if AwsPrefix != "" {
			conf.AwsPrefix = AwsPrefix
		}

		log.Info().Msg("======================= Start backup =======================")
		// Generate temp dir
		conf.TempBackupDir, err = ioutil.TempDir("", "spiderhouse")
		if err != nil {
			log.Fatal().Err(err).Msg("error creating temp dir")
		}
		log.Info().Msgf("Temp dir is : %s", conf.TempBackupDir)
		defer os.RemoveAll(conf.TempBackupDir)

		// Dump file from DATABASE_URL
		dumpFile, err := pgDump(conf.DatabaseUrl, &conf)
		if err != nil {
			log.Fatal().Err(err).Msg("dump error")
		}

		if dumpFile != "" {

			err = uploadTos3(dumpFile, &conf, progress)
			if err != nil {
				log.Fatal().Err(err).Msg("upload error")
			}
		}
		log.Info().Msg("======================= End backup =======================")

		defer func() {
			err = os.RemoveAll(conf.TempBackupDir)
			if err != nil {
				log.Info().Msgf(" : %s", err)
			}
		}()
	},
}

func init() {
	rootCmd.AddCommand(captureCmd)
	captureCmd.PersistentFlags().Bool("progress", false, "Show upload progress")
	captureCmd.PersistentFlags().StringVarP(&AwsPrefix, "prefix", "p", "backups", "Aws Prefix")
}

type customReader struct {
	fp   *os.File
	size int64
	read int64
	bar  *pb.ProgressBar
}

func (r *customReader) Read(p []byte) (int, error) {
	return r.fp.Read(p)
}

func (r *customReader) ReadAt(p []byte, off int64) (int, error) {
	n, err := r.fp.ReadAt(p, off)
	if err != nil {
		return n, err
	}
	// Got the length have read( or means has uploaded), and you can construct your message
	atomic.AddInt64(&r.read, int64(n))
	// I have no idea why the read length need to be div 2,
	// maybe the request read once when Sign and actually send call ReadAt again
	// It works for me
	r.bar.Set(int(r.read / 2))

	return n, err
}

func (r *customReader) Seek(offset int64, whence int) (int64, error) {
	return r.fp.Seek(offset, whence)
}

func ensurepath(command string) (string, error) {
	_, err := exec.LookPath(command)
	if err != nil {
		return "", err
	}

	return command, nil
}

func pgDump(PgURL string, conf *Configuration) (string, error) {

	// Ensure pg_dump  is present
	PGDumpCmd, err := ensurepath("pg_dump")
	if err != nil {
		return "", err
	}

	var connectionOptions []string
	connectionOptions = append(connectionOptions, "-Fc", PgURL)

	cmd := exec.Command(PGDumpCmd, connectionOptions...)

	// Parse Postgres URL to extract hostname and feed the logs with it
	// so you know where you are backuping from
	u, err := url.Parse(PgURL)
	if err != nil {
		return "", err
	}

	// Create backups directory if not exists
	_ = os.Mkdir(conf.TempBackupDir, 0700)

	// Create backup file
	location, err := time.LoadLocation("Europe/Paris")
	if err != nil {
		return "", err
	}
	filename := fmt.Sprintf(`production-backup-%v.dump`, time.Now().In(location).Format("2006-01-02-150405"))
	filepath := filepath.Join(conf.TempBackupDir, filename)
	tmpfile, err := os.Create(filepath + ".partial")
	if err != nil {
		return "", errors.New(fmt.Sprintf("could not create tmp dump file %s", err))
	}
	defer func() {
		tmpfile.Close()
	}()

	// Create a writer to that file
	writer := bufio.NewWriter(tmpfile)
	defer writer.Flush()

	stderr, err := cmd.StderrPipe()
	if err != nil {
		return "", err
	}

	stdoutPipe, err := cmd.StdoutPipe()
	if err != nil {
		return "", err
	}

	// Start Command and proceed without waiting for it to complete
	err = cmd.Start()
	if err != nil {
		return "", err
	}
	start := time.Now()
	log.Info().Msgf("Start backup %v from %v to %v", filename, u.Host, conf.TempBackupDir)

	go io.Copy(writer, stdoutPipe)

	pgDumpErr, err := ioutil.ReadAll(stderr)
	if err != nil {
		return "", err
	}
	if len(pgDumpErr) > 0 {
		return "", errors.New(fmt.Sprintf("%s",pgDumpErr))
	}

	// Here we wait for the command to complete
	err = cmd.Wait()
	if err != nil {
		return "", err
	}

	elapsed := time.Since(start)
	log.Info().Msgf("Backup of %v completed in %s", filename, elapsed)

	err = os.Rename(filepath+".partial", filepath)
	if err != nil {
		return "", errors.New("Could not rename partial download")
	}
	return filename, nil
}

func uploadTos3(filename string, conf *Configuration, progress bool) error {

	dumpfile, err := os.Open(filepath.Join(conf.TempBackupDir, filename))
	if err != nil {
		return fmt.Errorf("Failed to open file", filename, err)
	}
	defer dumpfile.Close()

	dumpFileInfo, err := dumpfile.Stat()
	if err != nil {
		return err
	}

	// Create the progress bar outside the go routine to avoid glitches
	// The progress bar is created and stopped outside the goroutine but started inside.
	bar := pb.New(int(dumpFileInfo.Size())).SetRefreshRate(time.Millisecond)
	bar.ShowFinalTime = false
	bar.ShowCounters = false
	bar.ShowTimeLeft = false
	bar.SetMaxWidth(100)

	reader := &customReader{
		fp:   dumpfile,
		size: dumpFileInfo.Size(),
		bar:  bar,
	}

	//select Region to use.
	awsconf := aws.Config{Region: aws.String(conf.AwsRegion)}

	sess, err := session.NewSession(&awsconf)
	if err != nil {
		log.Info().Msgf("Error getting new session : %s", err)
	}

	uploader := s3manager.NewUploader(sess, func(u *s3manager.Uploader) {
		u.PartSize = 5 * 1024 * 1024
		u.LeavePartsOnError = true
	})

	log.Info().Msgf("Uploading %v to %v...", filename, filepath.Join(conf.AwsBucket, conf.AwsPrefix, filename))
	if progress {
		bar.Start()
	}
	result, err := uploader.Upload(&s3manager.UploadInput{
		Bucket: aws.String(conf.AwsBucket),
		Key:    aws.String(filepath.Join(conf.AwsPrefix, filename)),
		Body:   reader,
	})
	if err != nil {
		return err
	}
	if progress {
		bar.Finish()
	}

	if debug {
		log.Info().Msgf("Successfully uploaded %s to %s\n", filename, result.Location)
	} else {
		log.Info().Msgf("Successfully uploaded %s to %s\n", filename, conf.AwsPrefix)
	}

	// pretext := "👋 Hello, backup and upload to s3 successfull, I keep going 😎 "
	// attachment := Attachment{}
	// attachment.Title = filename
	// attachment.TitleLink = result.Location
	// attachment.Text = fmt.Sprintf("Size : %d", dumpFileInfo.Size())
	// attachment.PreText = pretext
	// attachment.Color = "#7CD197"
	// pingSlackWithAttachment("", attachment)

	log.Info().Msgf("✅  Backup and upload to s3 successfull : %s, %s", filename, fmt.Sprintf("Size : %d", dumpFileInfo.Size()))
	return nil
}
