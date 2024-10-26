package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"

	"github.com/alecthomas/kong"
	"github.com/go-logr/logr"
	"github.com/web-seven/overlock/cmd/overlock/configuration"
	"github.com/web-seven/overlock/cmd/overlock/environment"
	"github.com/web-seven/overlock/cmd/overlock/generate"
	"github.com/web-seven/overlock/cmd/overlock/provider"
	"github.com/web-seven/overlock/cmd/overlock/version"
	"github.com/web-seven/overlock/internal/kube"
	"github.com/web-seven/overlock/internal/namespace"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"

	"github.com/web-seven/overlock/cmd/overlock/registry"
	"github.com/web-seven/overlock/cmd/overlock/resource"
	"github.com/willabides/kongplete"
	"k8s.io/client-go/dynamic"
	ctrl "sigs.k8s.io/controller-runtime"
)

type Globals struct {
	Debug     bool        `short:"D" help:"Enable debug mode"`
	Version   VersionFlag `name:"version" help:"Print version information and quit"`
	Namespace string      `name:"namespace" help:"Namespace used for cluster resources"`
}

type VersionFlag string

func (v VersionFlag) Decode(ctx *kong.DecodeContext) error { return nil }
func (v VersionFlag) IsBool() bool                         { return true }
func (v VersionFlag) BeforeApply(app *kong.Kong, vars kong.Vars) error {
	fmt.Println(vars["version"])
	app.Exit(0)
	return nil
}

func getDescriptionText() string {
	bText := "Overlock CLI.\n\n"
	bText += "For more details open https://overlock.io \n\n"
	return bText
}

func (c *cli) AfterApply(ctx *kong.Context) error { //nolint:unparam
	config, _ := ctrl.GetConfig()
	if config != nil {
		ctx.Bind(config)
		dynamicClient, _ := dynamic.NewForConfig(config)
		kubeClient, _ := kube.Client(config)
		ctx.Bind(dynamicClient)
		ctx.Bind(kubeClient)
	}

	cfg := zap.NewDevelopmentConfig()
	cfg.Level = zap.NewAtomicLevelAt(zapcore.InfoLevel)
	cfg.EncoderConfig.TimeKey = ""
	cfg.EncoderConfig.CallerKey = ""
	if c.Globals.Debug {
		cfg.Level = zap.NewAtomicLevelAt(zapcore.DebugLevel)
	}

	if os.Getenv(namespace.OVERLOCK_ENGINE_NAMESPACE) != "" {
		namespace.Namespace = os.Getenv(namespace.OVERLOCK_ENGINE_NAMESPACE)
	} else if c.Globals.Namespace != "" {
		namespace.Namespace = c.Globals.Namespace
	}

	logger, _ := cfg.Build()
	ctrl.SetLogger(logr.Logger{})
	ctx.Bind(logger.Sugar())
	return nil
}

type cli struct {
	Globals

	Help               helpCmd                      `cmd:"" help:"Show help."`
	Environment        environment.Cmd              `cmd:"" name:"environment" aliases:"env" help:"Overlock Environment commands"`
	Configuration      configuration.Cmd            `cmd:"" name:"configuration" aliases:"cfg" help:"Overlock Configuration commands"`
	Resource           resource.Cmd                 `cmd:"" name:"resource" aliases:"res" help:"Overlock Resource commands"`
	Registry           registry.Cmd                 `cmd:"" name:"registry" aliases:"reg" help:"Packages registy commands"`
	InstallCompletions kongplete.InstallCompletions `cmd:"" help:"Install shell completions"`
	Provider           provider.Cmd                 `cmd:"" name:"provider" help:"Overlock Provider commands"`
	Search             registry.SearchCmd           `cmd:"" help:"Search for packages"`
	Generate           generate.Cmd                 `cmd:"" help:"Generate example by XRD YAML file"`
}

type helpCmd struct{}

func main() {

	c := cli{
		Globals: Globals{
			Version: VersionFlag(version.Version),
		},
	}

	parser := kong.Must(&c,
		kong.Name("overlock"),
		kong.Description(getDescriptionText()),
		kong.Help(func(options kong.HelpOptions, ctx *kong.Context) error {
			return kong.DefaultHelpPrinter(options, ctx)
		}),
		kong.Vars{
			"version": version.Version,
		},
		kong.ConfigureHelp(kong.HelpOptions{
			Tree: true,
		}))

	if len(os.Args) == 1 {
		_, err := parser.Parse([]string{"--help"})
		parser.FatalIfErrorf(err)
		return
	}

	kongCtx, err := parser.Parse(os.Args[1:])
	parser.FatalIfErrorf(err)

	ctx, cancel := context.WithCancel(context.Background())
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt)
	defer signal.Stop(sigCh)
	go func() {
		defer cancel()
		<-sigCh
		kongCtx.Exit(1)
	}()

	kongCtx.BindTo(ctx, (*context.Context)(nil))
	kongCtx.FatalIfErrorf(kongCtx.Run())
}