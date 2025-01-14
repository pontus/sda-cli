package main

import (
	"flag"
	"fmt"
	"os"

	createKey "github.com/NBISweden/sda-cli/create_key"
	"github.com/NBISweden/sda-cli/datasetsize"
	"github.com/NBISweden/sda-cli/decrypt"
	"github.com/NBISweden/sda-cli/download"
	"github.com/NBISweden/sda-cli/encrypt"
	"github.com/NBISweden/sda-cli/helpers"
	"github.com/NBISweden/sda-cli/list"
	"github.com/NBISweden/sda-cli/login"
	"github.com/NBISweden/sda-cli/upload"
	"github.com/NBISweden/sda-cli/version"
	log "github.com/sirupsen/logrus"
)

var Version = "development"

var Usage = `USAGE: %s <command> [command-args]

This is a helper tool that can help with common tasks when interacting
with the Sensitive Data Archive (SDA).
`

// Map of the sub-commands, and their arguments and usage text strings
type commandInfo struct {
	args    *flag.FlagSet
	usage   string
	argHelp string
}

var Commands = map[string]commandInfo{
	"encrypt":     {encrypt.Args, encrypt.Usage, encrypt.ArgHelp},
	"createKey":   {createKey.Args, createKey.Usage, createKey.ArgHelp},
	"decrypt":     {decrypt.Args, decrypt.Usage, decrypt.ArgHelp},
	"download":    {download.Args, download.Usage, download.ArgHelp},
	"upload":      {upload.Args, upload.Usage, upload.ArgHelp},
	"datasetsize": {datasetsize.Args, datasetsize.Usage, datasetsize.ArgHelp},
	"list":        {list.Args, list.Usage, list.ArgHelp},
	"login":       {login.Args, login.Usage, login.ArgHelp},
	"version":     {version.Args, version.Usage, version.ArgHelp},
}

// Main does argument parsing, then delegates to one of the sub modules
func main() {

	log.SetLevel(log.WarnLevel)
	command, args := ParseArgs()

	var err error

	switch command {
	case "encrypt":
		err = encrypt.Encrypt(args)
	case "createkey", "createKey", "create-key":
		err = createKey.CreateKey(args)
	case "decrypt":
		err = decrypt.Decrypt(args)
	case "download":
		err = download.Download(args)
	case "upload":
		err = upload.Upload(args)
	case "datasetsize":
		err = datasetsize.DatasetSize(args)
	case "list":
		err = list.List(args)
	case "login":
		err = login.NewLogin(args)
	case "version":
		err = version.Version(Version)
	default:
		fmt.Fprintf(os.Stderr, "Unknown command: %s", command)
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
	}
}

// Parses the command line arguments into a command, and keep the rest of the
// arguments for the subcommand
func ParseArgs() (string, []string) {

	// Print usage if no arguments are provided
	if len(os.Args) < 2 {
		Help("help")
	}

	if os.Args[1] == "version" || os.Args[1] == "-v" || os.Args[1] == "--version" {
		if len(os.Args) != 2 {
			Help("version")
		}

		return "version", os.Args
	}

	// Extract `command` from arg 1, then remove it from the flag list.
	command := os.Args[1]
	os.Args = append(os.Args[:1], os.Args[2:]...)

	// If `command` is help-like, we print the help text and exit
	switch command {
	case "-h", "help", "-help", "--help":
		var subcommand string
		if len(os.Args) > 1 {
			subcommand = os.Args[1]
		} else {
			subcommand = "help"
		}
		Help(subcommand)
	}

	// list command can have no arguments since it can use the config from login
	// so we immediately return in that case
	if command == "list" {
		return command, os.Args
	}

	// If no arguments are provided to the subcommand, it's not gonna be valid,
	// so we print the subcommand help
	if len(os.Args) == 1 {
		Help(command)
	}

	return command, os.Args
}

// Prints the main usage string, and the global help or command help depending
// on the `command` arg.
func Help(command string) {

	info, isLegal := Commands[command]
	if isLegal {
		// print subcommand help
		fmt.Fprintf(os.Stderr, info.usage+"\n", os.Args[0])
		fmt.Fprintln(os.Stderr, "Command line arguments:")
		info.args.PrintDefaults()
		fmt.Fprintln(os.Stderr, info.argHelp)
	} else {
		if command != "help" {
			fmt.Fprintf(os.Stderr, "Unknown command: %s\n", command)
		}
		// print main help
		fmt.Fprintf(os.Stderr, Usage, os.Args[0])
		fmt.Fprintln(os.Stderr, "The tool can help with these actions:")
		for _, info := range Commands {

			subcommandUsage := helpers.FormatSubcommandUsage(info.usage)

			fmt.Fprint(os.Stderr, subcommandUsage)
		}
		fmt.Fprintf(os.Stderr,
			"Use '%s help <command>' to get help with subcommand flags.\n",
			os.Args[0])
	}

	os.Exit(1)
}
