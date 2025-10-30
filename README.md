# falcon
[![Build Status](https://travis-ci.org/hungneox/falcon.svg?branch=master)](https://travis-ci.org/hungneox/falcon)
## In development

Resumable download accelerator written in Golang

[![asciicast](https://asciinema.org/a/155077.png)](https://asciinema.org/a/155077)

# Installation

Install dependencies with Go Modules (Go 1.11+ is required)

```
go mod tidy
```

Manual build

```
./bin/buid
```

# Usage

```
falcon [cmd]
```

Here is a list of available commands:

```
Usage:
  falcon [command]

Available Commands:
  get         Download the given url
  help        Help about any command
  resume      Resume unfinished task
  tasks       Listing all unfinished tasks

Flags:
  -h, --help   help for falcon

Use "falcon [command] --help" for more information about a command.
```


```
./build/falcon get --help
```

```
Download the given url

Usage:
  falcon get [url] [flags]

Flags:
  -c, --connection int   The number of connections (default 4)
  -h, --help             help for get
```
# LICENSE

[MIT](LICENSE)
