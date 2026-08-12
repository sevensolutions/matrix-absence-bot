// Command matrix-absence-bot logs into your own Matrix account and sends an
// automatic "I'm away" reply to direct messages, at most once per sender per
// day. There is no schedule or toggle: you are "absent" for exactly as long
// as this process is running. Start it before you leave, Ctrl-C it when
// you're back.
package main

import (
	"context"
	"flag"
	"os"
	"os/signal"
	"sync"
	"syscall"

	"github.com/rs/zerolog"
	"maunium.net/go/mautrix"
	"maunium.net/go/mautrix/crypto/cryptohelper"
	"maunium.net/go/mautrix/event"
	"maunium.net/go/mautrix/id"
)

func main() {
	configPath := flag.String("config", "config.yaml", "path to config file")
	flag.Parse()

	log := zerolog.New(zerolog.NewConsoleWriter()).With().Timestamp().Logger()

	cfg, err := LoadConfig(*configPath)
	if err != nil {
		log.Fatal().Err(err).Msg("Failed to load config")
	}

	client, err := mautrix.NewClient(cfg.Homeserver, id.UserID(cfg.UserID), cfg.AccessToken)
	if err != nil {
		log.Fatal().Err(err).Msg("Failed to create client")
	}
	client.DeviceID = id.DeviceID(cfg.DeviceID)
	client.Log = log

	syncer := mautrix.NewDefaultSyncer()
	client.Syncer = syncer

	cryptoHelper, err := cryptohelper.NewCryptoHelper(client, []byte(cfg.PickleKey), cfg.CryptoDBPath)
	if err != nil {
		log.Fatal().Err(err).Msg("Failed to set up crypto helper")
	}
	ctx := context.Background()
	if err := cryptoHelper.Init(ctx); err != nil {
		log.Fatal().Err(err).Msg("Failed to initialize crypto helper")
	}
	client.Crypto = cryptoHelper
	defer cryptoHelper.Close()

	rateLimiter, err := NewRateLimiter(cfg.StatePath)
	if err != nil {
		log.Fatal().Err(err).Msg("Failed to load rate limiter state")
	}

	dmTracker := NewDMTracker()
	handler := NewHandler(client, dmTracker, rateLimiter, cfg.ReplyMessage, log)

	// Track completion of the first sync round-trip so we know once it's
	// safe to start reacting to messages (and can run the recovery-key
	// verification, which needs an initial sync to have happened).
	firstSyncChan := make(chan struct{})
	var closeOnce sync.Once
	syncer.OnSync(func(_ context.Context, _ *mautrix.RespSync, _ string) bool {
		closeOnce.Do(func() { close(firstSyncChan) })
		return true
	})

	syncer.OnEventType(event.EventMessage, handler.HandleMessage)
	syncer.OnEventType(event.AccountDataDirectChats, dmTracker.HandleAccountData)

	syncStopped := make(chan error, 1)
	go func() {
		syncStopped <- client.Sync()
	}()

	log.Info().Msg("Waiting for initial sync...")
	select {
	case <-firstSyncChan:
		log.Info().Msg("Initial sync complete")
	case err := <-syncStopped:
		log.Fatal().Err(err).Msg("Sync stopped before completing initial sync")
	}

	if err := dmTracker.Load(ctx, client); err != nil {
		log.Error().Err(err).Msg("Failed to load initial DM room list; will rely on live updates")
	}

	if cfg.RecoveryKey != "" {
		if err := verifyWithRecoveryKey(ctx, cryptoHelper, cfg.RecoveryKey); err != nil {
			log.Error().Err(err).Msg("Failed to verify device with recovery key; decryption of some rooms may not work until this device is verified from another client")
		} else {
			log.Info().Msg("Device verified via recovery key")
		}
	} else {
		log.Warn().Msg("No recovery_key configured; decryption may not work until this device is verified from another client")
	}

	handler.MarkFirstSyncDone()
	log.Info().Msg("Absence bot is running - auto-replying to direct messages")

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)

	select {
	case sig := <-sigCh:
		log.Info().Str("signal", sig.String()).Msg("Shutting down")
		client.StopSync()
	case err := <-syncStopped:
		if err != nil {
			log.Fatal().Err(err).Msg("Sync stopped unexpectedly")
		}
	}
}

// verifyWithRecoveryKey uses the account's SSSS recovery key to cross-sign
// this device, so other devices trust it and share room (megolm) keys with
// it - without this, encrypted DMs may never decrypt.
func verifyWithRecoveryKey(ctx context.Context, ch *cryptohelper.CryptoHelper, recoveryKey string) error {
	machine := ch.Machine()

	keyID, keyData, err := machine.SSSS.GetDefaultKeyData(ctx)
	if err != nil {
		return err
	}
	key, err := keyData.VerifyRecoveryKey(keyID, recoveryKey)
	if err != nil {
		return err
	}
	if err := machine.FetchCrossSigningKeysFromSSSS(ctx, key); err != nil {
		return err
	}
	if err := machine.SignOwnDevice(ctx, machine.OwnIdentity()); err != nil {
		return err
	}
	return machine.SignOwnMasterKey(ctx)
}
