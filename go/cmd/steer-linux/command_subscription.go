// SPDX-License-Identifier: GPL-3.0-or-later

package main

import (
	"context"
	"errors"
	"flag"
	"net/http"
	"time"

	linuxplatform "github.com/gsh20040816/steer/go/internal/platform/linux"
)

func runSubscription(args []string) error {
	if len(args) == 0 || (args[0] != "update" && args[0] != "status" && args[0] != "clean") {
		return errors.New("usage: steer subscription update|status|clean [flags]")
	}
	command := args[0]
	flags := flag.NewFlagSet("subscription", flag.ContinueOnError)
	configPath := flags.String("config", "/etc/steer/config.json", "canonical JSON configuration file")
	stateDirectory := flags.String("state-dir", "/var/lib/steer", "subscription snapshot directory")
	runDirectory := flags.String("run-dir", "/run/steer", "shared operation lock directory")
	id := flags.String("id", "", "only update this subscription ID")
	nodeID := flags.String("node", "", "subscription node ID for clean")
	if err := flags.Parse(args[1:]); err != nil {
		return err
	}
	if flags.NArg() != 0 {
		return errors.New("subscription subcommands accept flags only")
	}
	if command == "status" {
		statuses, err := linuxplatform.ReadSubscriptionStatus(*configPath, *stateDirectory)
		if err != nil {
			return err
		}
		writeJSON(struct {
			OK            bool                               `json:"ok"`
			Subscriptions []linuxplatform.SubscriptionStatus `json:"subscriptions"`
		}{true, statuses})
		return nil
	}
	if command == "clean" {
		if *id == "" || *nodeID == "" {
			return errors.New("subscription clean requires --id and --node")
		}
		err := withOperationLock(*runDirectory, func() error {
			var err error
			_, err = linuxplatform.CleanSubscriptionNode(*configPath, *stateDirectory, *id, *nodeID)
			return err
		})
		if err != nil {
			return err
		}
		statuses, err := linuxplatform.ReadSubscriptionStatus(*configPath, *stateDirectory)
		if err != nil {
			return err
		}
		writeJSON(struct {
			OK            bool                               `json:"ok"`
			Subscriptions []linuxplatform.SubscriptionStatus `json:"subscriptions"`
		}{true, statuses})
		return nil
	}
	err := withOperationLock(*runDirectory, func() error {
		var err error
		_, err = linuxplatform.UpdateConfiguredSubscriptions(context.Background(), &http.Client{Timeout: 30 * time.Second}, *configPath, *stateDirectory, *id)
		return err
	})
	if err != nil {
		return err
	}
	statuses, err := linuxplatform.ReadSubscriptionStatus(*configPath, *stateDirectory)
	if err != nil {
		return err
	}
	writeJSON(struct {
		OK            bool                               `json:"ok"`
		Subscriptions []linuxplatform.SubscriptionStatus `json:"subscriptions"`
	}{true, statuses})
	return nil
}
