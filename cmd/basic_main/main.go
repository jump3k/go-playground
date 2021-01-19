package main

import (
	"flag"
	"fmt"
	"os"
)

var usageStr = `
Usage: basic_main [options]

Server Options:
	-a, --addr <host>                Bind to host address (default: 0.0.0.0)
	-p, --port <port>                Use port for clients (default: 6666)

Common Options:
	-h, --help                       Show this message
	-v, --version                    Show version
`

func usage() {
	fmt.Printf("%s\n", usageStr)
	os.Exit(0)
}

func main() {
	exe := "basic-main"

	fs := flag.NewFlagSet(exe, flag.ExitOnError)
	fs.Usage = usage

	opts, err := configureOptions(fs, os.Args[1:])
	if err != nil {
		fmt.Println(err)
		return
	}

	fmt.Println(opts)
}

func configureOptions(fs *flag.FlagSet, args []string) (*Options, error) {
	opts := &Options{}

	var (
		showVersion bool
		showHelp    bool
	)

	fs.BoolVar(&showHelp, "h", false, "Show this message")
	fs.BoolVar(&showHelp, "help", false, "Show this message")
	fs.BoolVar(&showVersion, "v", false, "Show Version")
	fs.BoolVar(&showVersion, "version", false, "Show Version")
	fs.IntVar(&opts.Port, "p", 0, "port to listen on.")
	fs.IntVar(&opts.Port, "port", 0, "port to listen on.")
	fs.StringVar(&opts.Host, "a", "", "Network host to listen on.")
	fs.StringVar(&opts.Host, "addr", "", "Network host to listen on.")

	if err := fs.Parse(args); err != nil {
		return nil, err
	}

	if showVersion {
		printVersion()
		return nil, nil
	}

	if showHelp {
		usage()
		return nil, nil
	}

	return opts, nil
}

var VERSION string = "1.0.0"

func printVersion() {
	fmt.Printf("basic-main: v%s\n", VERSION)
	os.Exit(0)
}

type Options struct {
	Port int
	Host string
}
