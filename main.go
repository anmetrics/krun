package main

import (
	"fmt"
	"os"
	"strconv"
	"strings"
)

const version = "1.0.0"

func usage() {
	fmt.Print(colorize(bold+cyan, "krun") + colorize(gray, " v"+version) + " — minimal process manager via systemd\n\n")
	fmt.Println(colorize(bold, "Usage:"))
	fmt.Println("  krun start <name> --cmd \"<command>\" [options]")
	fmt.Println("  krun start <config.json>")
	fmt.Println("  krun stop <name|all>")
	fmt.Println("  krun restart <name|all>")
	fmt.Println("  krun reload <name|all>")
	fmt.Println("  krun remove <name|all>")
	fmt.Println("  krun list")
	fmt.Println("  krun info <name>")
	fmt.Println("  krun logs <name> [--lines N] [--nostream]")
	fmt.Println("  krun monit")
	fmt.Println("  krun env <name>")
	fmt.Println("  krun config <name>")
	fmt.Println("  krun flush [name|all]")
	fmt.Println("  krun save")
	fmt.Println("  krun resurrect")
	fmt.Println("  krun startup")
	fmt.Println("  krun unstartup")
	fmt.Println("  krun reset <name>")
	fmt.Println()
	fmt.Println(colorize(bold, "Start options:"))
	fmt.Println("  --cmd \"<command>\"       Command to run")
	fmt.Println("  --cwd <path>            Working directory (default: current)")
	fmt.Println("  --env KEY=VAL           Environment variable (repeatable)")
	fmt.Println("  --max-memory <size>     Memory limit (e.g. 512M, 1G)")
	fmt.Println("  --instances <N>         Number of instances")
	fmt.Println("  --log-file <path>       Redirect stdout to file")
	fmt.Println("  --error-file <path>     Redirect stderr to file")
	fmt.Println("  --log-dir <path>        Log directory (daily rotation: name-YYYY-MM-DD.log)")
	fmt.Println("  --interpreter <path>    Interpreter (e.g. python3, node)")
	fmt.Println("  --cron-restart <sched>  Systemd calendar schedule for restart")
	fmt.Println("  --no-autorestart        Disable auto-restart on crash")
}

type cliArgs struct {
	named      map[string]string
	envList    []string
	positional []string
}

func parseCLI(args []string) cliArgs {
	result := cliArgs{
		named: make(map[string]string),
	}

	singleFlags := map[string]bool{
		"--no-autorestart": true,
		"--nostream":       true,
	}

	for i := 0; i < len(args); i++ {
		arg := args[i]

		if singleFlags[arg] {
			result.named[arg] = "true"
			continue
		}

		if strings.HasPrefix(arg, "--") {
			if i+1 < len(args) {
				if arg == "--env" {
					result.envList = append(result.envList, args[i+1])
				} else {
					result.named[arg] = args[i+1]
				}
				i++
				continue
			}
		}

		result.positional = append(result.positional, arg)
	}
	return result
}

func buildAppConfig(args cliArgs) AppConfig {
	app := AppConfig{
		Cmd:         args.named["--cmd"],
		Cwd:         args.named["--cwd"],
		MaxMemory:   args.named["--max-memory"],
		LogFile:     args.named["--log-file"],
		ErrorFile:   args.named["--error-file"],
		LogDir:      args.named["--log-dir"],
		Interpreter: args.named["--interpreter"],
		CronRestart: args.named["--cron-restart"],
	}

	if len(args.positional) > 0 {
		app.Name = args.positional[0]
	}
	if n := args.named["--name"]; n != "" {
		app.Name = n
	}

	if args.named["--no-autorestart"] == "true" {
		f := false
		app.AutoRestart = &f
	}

	if n, err := strconv.Atoi(args.named["--instances"]); err == nil {
		app.Instances = n
	}

	if len(args.envList) > 0 {
		app.Env = make(map[string]string)
		for _, e := range args.envList {
			if idx := strings.Index(e, "="); idx > 0 {
				app.Env[e[:idx]] = e[idx+1:]
			}
		}
	}

	return app
}

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(1)
	}

	subcmd := os.Args[1]
	rest := os.Args[2:]
	args := parseCLI(rest)

	var err error

	switch subcmd {
	case "start":
		if len(args.positional) == 1 && strings.HasSuffix(args.positional[0], ".json") {
			err = cmdStartEcosystem(args.positional[0])
		} else {
			app := buildAppConfig(args)
			err = cmdStart(app)
		}

	case "stop":
		name := "all"
		if len(args.positional) > 0 {
			name = resolveName(args.positional[0])
		}
		err = cmdStop(name)

	case "restart":
		name := "all"
		if len(args.positional) > 0 {
			name = resolveName(args.positional[0])
		}
		err = cmdRestart(name)

	case "reload":
		name := "all"
		if len(args.positional) > 0 {
			name = resolveName(args.positional[0])
		}
		err = cmdReload(name)

	case "remove", "delete":
		if len(args.positional) < 1 {
			printError("usage: krun remove <name|id|all>")
			os.Exit(1)
		}
		err = cmdRemove(resolveName(args.positional[0]))

	case "list", "ls", "status":
		err = cmdList()

	case "info", "describe", "show":
		if len(args.positional) < 1 {
			printError("usage: krun info <name|id>")
			os.Exit(1)
		}
		err = cmdInfo(resolveName(args.positional[0]))

	case "logs", "log":
		if len(args.positional) < 1 {
			printError("usage: krun logs <name|id> [--lines N] [--nostream]")
			os.Exit(1)
		}
		lines, _ := strconv.Atoi(args.named["--lines"])
		noStream := args.named["--nostream"] == "true"
		err = cmdLogs(resolveName(args.positional[0]), lines, noStream)

	case "monit", "monitor", "dash":
		err = cmdMonit()

	case "env":
		if len(args.positional) < 1 {
			printError("usage: krun env <name|id>")
			os.Exit(1)
		}
		err = cmdEnv(resolveName(args.positional[0]))

	case "config":
		if len(args.positional) < 1 {
			printError("usage: krun config <name|id>")
			os.Exit(1)
		}
		err = cmdShowConfig(resolveName(args.positional[0]))

	case "flush":
		name := ""
		if len(args.positional) > 0 {
			name = resolveName(args.positional[0])
		}
		err = cmdFlush(name)

	case "save":
		err = cmdSave()

	case "resurrect":
		err = cmdResurrect()

	case "startup":
		err = cmdStartup()

	case "unstartup":
		err = cmdUnstartup()

	case "reset":
		if len(args.positional) < 1 {
			printError("usage: krun reset <name|id>")
			os.Exit(1)
		}
		err = cmdReset(resolveName(args.positional[0]))

	case "version", "-v", "--version":
		fmt.Println("krun v" + version)

	case "help", "-h", "--help":
		usage()

	default:
		printError("unknown command: %s", subcmd)
		fmt.Println()
		usage()
		os.Exit(1)
	}

	if err != nil {
		printError("%v", err)
		os.Exit(1)
	}
}
