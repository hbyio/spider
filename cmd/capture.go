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
	"log"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"sync/atomic"
	"time"
)

var conf Configuration

type Configuration struct {
	BackupDirPath      string `env:"BACKUP_DIR" env-default:"backups" env-description:"Where to temporary save backups before uploading to s3"`
	DatabaseUrl        string `env:"DATABASE_URL" env-default:"postgres://root:password@127.0.0.1:5432/devdb" env-description:"Source database url to pull backup from" env-required`
	SlackWebHook       string `env:"SLACK_WEBHOOK" env-description:"Slack webhook to report informations"`
	AwsBucket          string `env:"AWS_BUCKET" env-description:"Aws bucket to store dump files" env-required`
	AwsRegion          string `env:"AWS_REGION" env-description:"Aws bucket region" env-required`
	AwsAccessKeyId     string `env:"AWS_ACCESS_KEY_ID" env-description:"Aws access key id"`
	AwsSecretAccessKey string `env:"AWS_SECRET_ACCESS_KEY" env-description:"Aws secret access key"`
	BackupDirHourly    string `env:"PREFIX_HOURLY" env-default:"hourly" env-description:"Prefix on your bucket where to store hourly backups"`
	BackupDirDaily     string `env:"PREFIX_DAILY" env-default:"daily" env-description:"Prefix on your bucket where to store daily backups"`
	BackupDirMonthly   string `env:"PREFIX_MONTHLY" env-default:"monthly" env-description:"Prefix on your bucket where to store monthly backups"`
}

// captureCmd represents the capture command
var captureCmd = &cobra.Command{
	Use:   "capture",
	Short: "Capture a database dump and place it on s3",
	Long:  ``,
	RunE: func(cmd *cobra.Command, args []string) error {

		err := cleanenv.ReadEnv(&conf)
		if err != nil {
			return fmt.Errorf("Error reading env : %s", err)
		}
		progress, err := cmd.PersistentFlags().GetBool("progress")
		if err != nil {
			log.Printf("Error getting progress flag value : %s", err)
		}

		// Create (if not exists) logfile
		logfile, err := os.OpenFile("spiderhouse.log",
			os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
		if err != nil {
			log.Println(err)
		}
		defer logfile.Close()
		// Write logs to logfile
		// TODO : find a way to output to file AND stdOut
		//log.SetOutput(logfile)
		log.SetOutput(os.Stdout)
		// Enable line numbers in logging
		log.SetFlags(log.LstdFlags | log.Lshortfile)

		//log.Printf("%#v", conf)

		dumpFile, err := pgDump(conf.DatabaseUrl, &conf)
		if err != nil {
			return fmt.Errorf("Dump error : %s", err)
		}
		if dumpFile != "" {

			err = uploadTos3(dumpFile, &conf, progress)
			if err != nil {
				log.Printf("Error during upload : %s", err)
			}

			err = os.RemoveAll(conf.BackupDirPath)
			if err != nil {
				log.Printf("Could not remove backup file %v : %v", dumpFile, err)
			}
			if _, err := os.Stat(conf.BackupDirPath); os.IsNotExist(err) {
				log.Printf("%s successfully removed", conf.BackupDirPath)
			} else {
				log.Printf("%s was not removed", conf.BackupDirPath)
			}

		}

		return nil
	},
}

func init() {
	rootCmd.AddCommand(captureCmd)

	// Here you will define your flags and configuration settings.

	// Cobra supports Persistent Flags which will work for this command
	// and all subcommands, e.g.:
	captureCmd.PersistentFlags().Bool("progress", false, "Show upload progress")

	// Cobra supports local flags which will only run when this command
	// is called directly, e.g.:
	// captureCmd.Flags().BoolP("toggle", "t", false, "Help message for toggle")
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

func ensurepath(command string) string {
	_, LookErr := exec.LookPath(command)
	if LookErr != nil {
		log.Fatalf("%v is not found", command)
		panic(LookErr)
	}
	//log.Printf("command '%v' found at %v", command, ensuredcommand)
	return command
}

func pgDump(PgURL string, conf *Configuration) (string, error) {

	// Ensure pg_dump  is present
	PGDumpCmd := ensurepath("pg_dump")

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
	_ = os.Mkdir(conf.BackupDirPath, 0700)

	// Create backup file
	filename := fmt.Sprintf(`production-backup-%v.dump`, time.Now().Local().Format("2006-01-02-150405"))
	filepath := filepath.Join(conf.BackupDirPath, filename)
	tmpfile, err := os.Create(filepath + ".partial")
	if err != nil {
		return "", errors.New("Could not create tmp dump file")
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

	// Start Command and proceed without wainting for it to complete
	err = cmd.Start()
	start := time.Now()
	log.Printf("Start backup %v from %v to %v", filename, u.Host, conf.BackupDirPath)

	go io.Copy(writer, stdoutPipe)

	pgDumpErr, _ := ioutil.ReadAll(stderr)
	if len(pgDumpErr) > 0 {
		log.Printf("%s", pgDumpErr)
	}

	// Here we wait for the command to complete
	err = cmd.Wait()
	if err != nil {
		return "", err
	}

	elapsed := time.Since(start)
	log.Printf("Backup of %v completed in %s", filename, elapsed)

	err = os.Rename(filepath+".partial", filepath)
	if err != nil {
		return "", errors.New("Could not rename partial download")
	}
	return filename, nil
}

func uploadTos3(filename string, conf *Configuration, progress bool) error {

	dumpfile, err := os.Open(filepath.Join(conf.BackupDirPath, filename))
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
		log.Printf("Error getting new session : %s", err)
	}

	uploader := s3manager.NewUploader(sess, func(u *s3manager.Uploader) {
		u.PartSize = 5 * 1024 * 1024
		u.LeavePartsOnError = true
	})

	log.Printf("Uploading %v to %v...", filename, filepath.Join(conf.AwsBucket, conf.BackupDirHourly, filename))
	if progress {
		bar.Start()
	}
	result, err := uploader.Upload(&s3manager.UploadInput{
		Bucket: aws.String(conf.AwsBucket),
		Key:    aws.String(filepath.Join(conf.BackupDirHourly, filename)),
		Body:   reader,
	})
	if err != nil {
		return err
	}
	if progress {
		bar.Finish()
	}

	log.Printf("Successfully uploaded %s to %s\n", filename, result.Location)

	dayOfMonth := time.Now().Day()
	hourOfDay := time.Now().Hour()
	if hourOfDay == 6 {
		dailyresult, err := uploader.Upload(&s3manager.UploadInput{
			Bucket: aws.String(conf.AwsBucket),
			Key:    aws.String(filepath.Join(conf.BackupDirDaily, filename)),
			Body:   reader,
		})
		if err != nil {
			return fmt.Errorf("Could not upload to daily folder : %v", err)
		}
		log.Printf("Successfully uploaded %s to %s\n", filename, dailyresult.Location)
	}
	if dayOfMonth == 1 {
		monthlyresult, err := uploader.Upload(&s3manager.UploadInput{
			Bucket: aws.String(conf.AwsBucket),
			Key:    aws.String(filepath.Join(conf.BackupDirMonthly, filename)),
			Body:   reader,
		})
		if err != nil {
			return fmt.Errorf("Could not upload to monthly folder : %v", err)
		}
		log.Printf("Successfully uploaded %s to %s\n", filename, monthlyresult.Location)
	}

	// pretext := "👋 Hello, backup and upload to s3 successfull, I keep going 😎 "
	// attachment := Attachment{}
	// attachment.Title = filename
	// attachment.TitleLink = result.Location
	// attachment.Text = fmt.Sprintf("Size : %d", dumpFileInfo.Size())
	// attachment.PreText = pretext
	// attachment.Color = "#7CD197"
	// pingSlackWithAttachment("", attachment)

	log.Printf("👋 Hello, backup and upload to s3 successfull, I keep going 😎 : %s, %s", filename, fmt.Sprintf("Size : %d", dumpFileInfo.Size()))
	return nil
}
