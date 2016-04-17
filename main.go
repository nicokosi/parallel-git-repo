package main

import (
	"bytes"
	"flag"
	"fmt"
	"github.com/codegangsta/cli"
	"github.com/jcgay/parallel-git-repo/command"
	"github.com/mitchellh/go-homedir"
	"gopkg.in/yaml.v2"
	"io"
	"io/ioutil"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
)

var VERSION = "unknown-snapshot"

var (
	quiet        bool
	printVersion bool
)

const help = `NAME:
  Parallel Git Repositories - Execute commands on multiple Git repositories in parallel!

USAGE:
  ./parallel-git-repo [global options] command [arguments...]

VERSION:
  %s

COMMANDS:
%s
GLOBAL OPTIONS:
  -h	show help
  -v	print the version
`

func main() {
	home, err := homedir.Dir()
	if err != nil {
		log.Fatalf("Cannot read user home directory.\n%v", err)
	}
	configuration := NewConfiguration(home)

	flag.BoolVar(&quiet, "q", false, "do not print stdout commands result, only stderr will be shown")
	flag.BoolVar(&printVersion, "v", false, "print current version")

	flag.Usage = func() {
		fmt.Fprint(os.Stderr, fmt.Sprintf(help, VERSION, listCommands(configuration)))
		flag.PrintDefaults()
	}

	flag.Parse()

	if printVersion {
		fmt.Printf("version: %s", VERSION)
		os.Exit(0)
	}

	run(configuration, withoutFlags(os.Args[1:]))
}

func listCommands(config *Configuration) string {
	commands := config.ListCommands()

	maxSize := 3
	for _, value := range commands {
		if size := len(value); size > maxSize {
			maxSize = size
		}
	}

	result := fmt.Sprintf("  %-"+strconv.Itoa(maxSize)+"s	%s\n", "run", "run an arbitrary command")
	for key, value := range commands {
		result += fmt.Sprintf("  %-"+strconv.Itoa(maxSize)+"s	%s\n", key, value)
	}

	return result
}

func run(config *Configuration, args []string) {
	commandName := args[0]
	var toExec []string
	if commandName == "run" {
		toExec = args[1:]
	} else {
		toExec = strings.Split(config.ListCommands()[commandName], " ")
	}

	runner := NewRunner(&command.Run{ToExec: toExec, Quiet: quiet}, config)
	runner.Run(args[1:])
}

func withoutFlags(args []string) []string {
	for i, value := range args {
		if !strings.HasPrefix(value, "-") {
			return args[i:]
		}
	}
	return args
}

type Repositories interface {
	ListRepositories() []string
}

type Commands interface {
	ListCommands() map[string]string
}

type Configuration struct {
	content map[interface{}]interface{}
}

func NewConfiguration(homeDir string) *Configuration {
	data, err := ioutil.ReadFile(homeDir + "/.parallel-git-repositories")
	if err != nil {
		log.Fatalf("Can't read configuration file %s/.parallel-git-repositories, verify that the file is correctly set...\n%v", homeDir, err)
	}

	config := make(map[interface{}]interface{})
	if err := yaml.Unmarshal(data, &config); err != nil {
		log.Fatalf("Configuration file %s/.parallel-git-repositories is not a valid yaml file.\n%v", homeDir, err)
	}

	return &Configuration{config}
}

func (config *Configuration) ListRepositories() []string {
	all := config.content["repositories"].([]interface{})
	result := make([]string, len(all))
	for i, repo := range all {
		result[i] = repo.(string)
	}
	return result
}

func (config *Configuration) ListCommands() map[string]string {
	all := config.content["commands"].(map[interface{}]interface{})
	result := make(map[string]string)
	for key, value := range all {
		result[key.(string)] = value.(string)
	}
	return result
}

type RunnableCommand interface {
	Executable() string
	Options() []string
	Output(output string, errOutput string, err error) string
}

type Runner struct {
	RunnableCommand

	repos  Repositories
	writer io.Writer
}

func NewRunner(command RunnableCommand, repos Repositories) *Runner {
	return &Runner{
		RunnableCommand: command,
		repos:           repos,
		writer:          os.Stdout,
	}
}

func (runner *Runner) Run(args cli.Args) {
	wg := sync.WaitGroup{}
	for _, repo := range runner.repos.ListRepositories() {
		wg.Add(1)
		go func(repo string) {
			defer wg.Done()
			output := new(bytes.Buffer)
			errorOutput := new(bytes.Buffer)

			command := exec.Command(runner.RunnableCommand.Executable(), forwardArgs(runner.RunnableCommand.Options(), args)...)
			command.Stdout = output
			command.Stderr = errorOutput
			command.Dir = repo
			err := command.Run()

			fmt.Fprintln(runner.writer, filepath.Base(repo)+": "+runner.RunnableCommand.Output(strings.TrimSpace(output.String()), strings.TrimSpace(errorOutput.String()), err))
		}(repo)
	}
	wg.Wait()
}

var option = regexp.MustCompile(`\$([0-9]+)`)

func forwardArgs(opts []string, args cli.Args) []string {
	result := make([]string, 0)
	for _, opt := range opts {
		if opt == "$@" {
			result = append(result, args...)
		} else if option.MatchString(opt) {
			result = append(result, option.ReplaceAllStringFunc(opt, func(substring string) string {
				index, _ := strconv.Atoi(substring[1:])
				return args[index-1]
			}))
		} else {
			result = append(result, opt)
		}
	}
	return result
}
