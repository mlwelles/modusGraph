// Package migratecli provides reusable Kong command structs for schema
// migrations. Mount MigrateCmd in any Kong CLI to get
// "migrate up / down / status / version".
//
// This package intentionally does NOT import Kong: the command structs carry
// only inert Kong struct tags, and dispatch happens through the consumer's own
// kong.New(..., kong.Bind(provider)) call. That keeps the modusgraph library
// free of a CLI-framework dependency.
package migratecli

import (
	"context"
	"fmt"

	mg "github.com/matthewmcneely/modusgraph"
	"github.com/matthewmcneely/modusgraph/migrate"
)

// Provider supplies a Client and the registered migration list at runtime.
// Implement this in your application and bind it via kong.Bind so each
// sub-command's Run method receives it.
type Provider interface {
	Client() mg.Client
	Migrations() []migrate.Migration
}

// MigrateCmd is the parent command. Mount it in your CLI struct:
//
//	type CLI struct {
//	    Migrate migratecli.MigrateCmd `cmd:"" help:"Manage schema migrations."`
//	}
//
// then run kong.New(&cli, kong.Bind(provider)).
type MigrateCmd struct {
	Up      UpCmd      `cmd:"" help:"Apply all pending migrations."`
	Down    DownCmd    `cmd:"" help:"Roll back migrations to a target version."`
	Status  StatusCmd  `cmd:"" help:"Show applied and pending migrations."`
	Version VersionCmd `cmd:"" help:"Print the current migration version."`
}

// UpCmd applies pending migrations.
type UpCmd struct{}

func (c *UpCmd) Run(p Provider) error {
	return migrate.Run(context.Background(), p.Client(), p.Migrations())
}

// DownCmd rolls back to a target version ID.
type DownCmd struct {
	To int64 `arg:"" name:"version" help:"Roll back migrations applied after this version ID (0 = roll back all)."`
}

func (c *DownCmd) Run(p Provider) error {
	return migrate.Down(context.Background(), p.Client(), p.Migrations(), c.To)
}

// StatusCmd prints applied and pending migration state.
type StatusCmd struct{}

func (c *StatusCmd) Run(p Provider) error {
	result, err := migrate.Status(context.Background(), p.Client(), p.Migrations())
	if err != nil {
		return err
	}
	fmt.Printf("Applied (%d):\n", len(result.Applied))
	for _, e := range result.Applied {
		drift := ""
		if e.HasDrift {
			drift = " *** SCHEMA DRIFT DETECTED ***"
		}
		fmt.Printf("  [x] %d  %s%s\n", e.ID, e.Name, drift)
	}
	fmt.Printf("In progress (%d):\n", len(result.InProgress))
	for _, e := range result.InProgress {
		fmt.Printf("  [~] %d  %s (%d/%d steps applied — resumes on next up)\n", e.ID, e.Name, e.StepsApplied, e.StepsTotal)
	}
	fmt.Printf("Pending (%d):\n", len(result.Pending))
	for _, e := range result.Pending {
		fmt.Printf("  [ ] %d  %s\n", e.ID, e.Name)
	}
	return nil
}

// VersionCmd prints the highest applied migration ID.
type VersionCmd struct{}

func (c *VersionCmd) Run(p Provider) error {
	v, err := migrate.Version(context.Background(), p.Client())
	if err != nil {
		return err
	}
	if v == 0 {
		fmt.Println("no migrations applied")
	} else {
		fmt.Printf("version: %d\n", v)
	}
	return nil
}
