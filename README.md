# Spiderhouse

Spiderhouse is a Golang executable with one obsession : capture a database dump and store it safely in its Aws S3 house.

## Installation

Right now you will have to build from source to install

```shell
git clone https://github.com/hbyio/spiderhouse
cd spiderhouse
go get -u
go install
```

## Usage

```shell
Spiders catch your dumps from database_url and store them on their s3 house

Usage:
  spiderhouse [command]

Available Commands:
  capture     Capture a database dump and place it on s3
  explain     Print env variables descriptions
  help        Help about any command

Flags:
  -h, --help     help for spiderhouse
  -t, --toggle   Help message for toggle

Use "spiderhouse [command] --help" for more information about a command.

```

## Contributing
Pull requests are welcome. For major changes, please open an issue first to discuss what you would like to change.

## License
[MIT](https://choosealicense.com/licenses/mit/)