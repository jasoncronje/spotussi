// Command spotussi is the single binary: HTTP server, API, embedded UI and
// (in later milestones) background capture workers.
//
// Usage:
//
//	spotussi [serve] [--mock] [--mock-size full|small|tiny] [--mock-seed N]
//	spotussi health           # exit 0 if the local server answers /api/v1/health
//	spotussi reset-password   # forget the owner password; set a new one in the UI
//	spotussi import PATH...   # import Extended Streaming History exports (zip, JSON file or
//	                          # unpacked directory) into the database in DATA_DIR; safe while
//	                          # the server runs
//	spotussi backup FILE|DIR  # consistent snapshot of the database, safe while the server runs
//	spotussi restore [--key KEY] FILE  # replace the database with a backup (server stopped)
//	spotussi mcp --url URL [--token TOKEN]
//	                          # stdio MCP server for clients that only speak stdio: proxies to
//	                          # URL/mcp with an API token (or SPOTUSSI_URL, SPOTUSSI_TOKEN)
//	spotussi version
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"runtime/debug"
	"strconv"
	"strings"
	"sync/atomic"
	"syscall"
	"time"
	_ "time/tzdata" // IANA zones even in images without /usr/share/zoneinfo

	"spotussi/internal/api"
	"spotussi/internal/auth"
	"spotussi/internal/capture"
	"spotussi/internal/config"
	"spotussi/internal/db"
	"spotussi/internal/derive"
	"spotussi/internal/hydrate"
	"spotussi/internal/importer"
	"spotussi/internal/keyring"
	"spotussi/internal/live"
	"spotussi/internal/mcp"
	"spotussi/internal/metadata"
	"spotussi/internal/mock"
	"spotussi/internal/refdata"
	"spotussi/internal/spotify"
	"spotussi/web"
)

// version is set at build time with -ldflags "-X main.version=...".
var version = "dev"

func main() {
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo})))
	args := os.Args[1:]
	cmd := "serve"
	if len(args) > 0 && args[0] != "" && args[0][0] != '-' {
		cmd, args = args[0], args[1:]
	}
	var err error
	switch cmd {
	case "serve":
		err = serve(args)
	case "health":
		err = health()
	case "reset-password":
		err = resetPassword()
	case "import":
		err = importCmd(args)
	case "mcp":
		err = mcpCmd(args)
	case "backup":
		err = backupCmd(args)
	case "restore":
		err = restoreCmd(args)
	case "version":
		fmt.Println(version)
	default:
		err = fmt.Errorf("unknown command %q (want serve, health, reset-password, import, backup, restore, mcp or version)", cmd)
	}
	if err != nil {
		slog.Error(err.Error())
		os.Exit(1)
	}
}

func serve(args []string) error {
	fs := flag.NewFlagSet("serve", flag.ContinueOnError)
	mockMode := fs.Bool("mock", false, "serve the synthetic reference dataset from memory instead of the database")
	mockSize := fs.String("mock-size", "full", "mock dataset size: full (15y, ~500k plays), small (~50k) or tiny (~5k)")
	mockSeed := fs.Uint64("mock-seed", 1, "mock dataset seed")
	mockTZ := fs.String("mock-tz", "", "listener timezone for the mock dataset (default: this machine's zone)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if *mockMode {
		// Mock mode stays login-free and stateless unless asked otherwise:
		// the UI is developed against it.
		if !cfg.AuthModeSet {
			cfg.AuthMode = config.AuthNone
		}
		if !cfg.DataDirSet {
			dir, err := os.MkdirTemp("", "spotussi-mock-")
			if err != nil {
				return err
			}
			defer os.RemoveAll(dir)
			cfg.DataDir = dir
		}
	}
	database, err := db.Open(ctx, cfg.DataDir)
	if err != nil {
		return err
	}
	defer database.Close()
	slog.Info("database ready", "path", database.Path)
	keys, err := keyring.LoadOrCreate(cfg.DataDir)
	if err != nil {
		return err
	}

	authSvc := auth.New(auth.Options{DB: database, Keyring: keys, Mode: cfg.AuthMode,
		ProxyHeader: cfg.ProxyAuthHeader, Proxies: cfg.TrustedProxies, AnnounceSetupToken: announceSetupToken})
	// Prints the first-run setup token now if no owner password exists (D21).
	if _, err := authSvc.SetupRequired(ctx); err != nil {
		return err
	}
	spotifyClient := spotify.New(spotify.Options{DB: database, Keyring: keys,
		Limiter: spotify.NewLimiter(spotify.LimiterConfig{Rate: cfg.SpotifyRate, Burst: cfg.SpotifyBurst,
			LiveReserve: cfg.SpotifyLiveReserve}, nil)})
	opts := api.Options{Version: version, Mode: "live", UI: web.UI(), DB: database,
		Auth: authSvc, Spotify: spotifyClient,
		BaseURL: cfg.BaseURL, Proxies: cfg.TrustedProxies,
		LoopbackPort:   spotify.LoopbackPort{Default: cfg.Port, Override: cfg.SpotifyLoopbackPort},
		ImportDir:      filepath.Join(cfg.DataDir, "imports"),
		ImportMaxBytes: cfg.ImportMaxBytes,
	}
	if cfg.AuthMode == config.AuthNone && !*mockMode {
		slog.Warn("AUTH_MODE=none: anyone who can reach this server can read and change it")
	}
	if !*mockMode {
		// The derivation engine (M5) exists once the backend is built,
		// after the services that feed it; their hooks reach it here.
		var engine atomic.Pointer[derive.Engine]
		rawChanged := func() {
			if e := engine.Load(); e != nil {
				e.RawChanged()
			}
		}
		liveChanged := func() {
			if e := engine.Load(); e != nil {
				e.LiveChanged()
			}
		}
		// Catalog hydration (M4): seeds the catalog from live capture,
		// queues export tracks most-played first and looks them up in the
		// limiter's background lane.
		var hyd *hydrate.Hydrator
		catalogChanged := func() {
			if e := engine.Load(); e != nil {
				e.CatalogChanged()
			}
		}
		captured, imported := func() {}, func() {}
		if cfg.HydrateEnabled {
			// Fallback sources (D27), each with its own limiter.
			var fb hydrate.Fallbacks
			ua := metadata.UserAgent(version, cfg.MetadataContact)
			if cfg.MetadataOEmbed {
				fb.OEmbed = metadata.NewOEmbed("", ua, nil, nil)
			}
			if cfg.MetadataDeezer {
				fb.Deezer = metadata.NewDeezer("", ua, nil, nil)
			}
			if cfg.MetadataListenBrainz {
				fb.ListenBrainz = metadata.NewListenBrainz("", "", ua, nil, nil)
			}
			if cfg.MetadataMusicBrainz {
				fb.MusicBrainz = metadata.NewMusicBrainz("", ua, nil, nil)
			}
			// HYDRATE_SPOTIFY_LOOKUPS=false: no Spotify catalog calls at all;
			// seeding and the fallbacks still run.
			var lookups hydrate.Spotify = spotifyClient
			if !cfg.HydrateSpotifyLookups {
				lookups = nil
				slog.Warn("HYDRATE_SPOTIFY_LOOKUPS=false: not looking up catalog entries on Spotify; fallback sources only")
			}
			hyd = hydrate.New(hydrate.Options{DB: database, Spotify: lookups, Fallbacks: fb,
				LimiterStats:     spotifyClient.LimiterStats,
				OnCatalogChanged: catalogChanged})
			captured, imported = hyd.Captured, hyd.Imported
			spotifyClient.OnConnect(hyd.Wake)
		} else {
			slog.Warn("HYDRATE_ENABLED=false: not fetching catalog metadata")
		}
		// Uploaded exports (M3): one import at a time, queued in the DB.
		imports, err := importer.NewService(ctx, importer.Options{DB: database, Dir: opts.ImportDir, OnFinished: imported, OnNewRows: rawChanged})
		if err != nil {
			return err
		}
		impCtx, impCancel := context.WithCancel(ctx)
		impDone := make(chan struct{})
		go func() { imports.Run(impCtx); close(impDone) }()
		defer func() { impCancel(); <-impDone }()
		lo := live.Options{DB: database, DefaultTimezone: localZoneName(), LimiterStats: spotifyClient.LimiterStats,
			Imports: imports}
		if hyd != nil {
			lo.Catalog = hyd.Status
			// Stopped (and waited for) before the database closes.
			hydCtx, hydCancel := context.WithCancel(ctx)
			hydDone := make(chan struct{})
			go func() { hyd.Run(hydCtx); close(hydDone) }()
			defer func() { hydCancel(); <-hydDone }()
		}
		if cfg.CaptureEnabled {
			// Live capture (M2): player poller + recently-played backstop.
			svc, err := capture.New(ctx, capture.Options{DB: database, Spotify: spotifyClient,
				Intervals:      capture.Intervals{Playing: cfg.CapturePlayingInterval, Idle: cfg.CaptureIdleInterval},
				RecentInterval: cfg.CaptureRecentInterval, OnStored: captured, OnNewRows: liveChanged})
			if err != nil {
				return err
			}
			lo.Capture = svc
			// Poll and backfill right after connecting rather than
			// waiting out the idle and backstop intervals.
			spotifyClient.OnConnect(svc.Wake)
			// Stopped (and waited for) before the database closes.
			capCtx, capCancel := context.WithCancel(ctx)
			done := make(chan struct{})
			go func() { svc.Run(capCtx); close(done) }()
			defer func() { capCancel(); <-done }()
			slog.Info("live capture running")
		} else {
			slog.Warn("CAPTURE_ENABLED=false: not recording listening")
		}
		be := live.New(lo)
		engine.Store(be.Engine())
		// Derivation (M5): loads or rebuilds plays at startup, then
		// follows imports, capture, settings and catalog changes. The
		// hydration worker (M4) reports catalog writes with
		// be.Engine().CatalogChanged().
		drvCtx, drvCancel := context.WithCancel(ctx)
		drvDone := make(chan struct{})
		go func() { be.Run(drvCtx); close(drvDone) }()
		defer func() { drvCancel(); <-drvDone }()
		opts.Backend = be
	}
	// Mock scenarios (latency, failures) wrap the API in mock mode only.
	wrap := func(h http.Handler) http.Handler { return h }
	if *mockMode {
		size, err := refdata.ParseSize(*mockSize)
		if err != nil {
			return err
		}
		start := time.Now()
		tz := *mockTZ
		if tz == "" {
			tz = localZoneName()
		}
		ds := refdata.Generate(refdata.Config{Size: size, Seed: *mockSeed, End: time.Now(), Timezone: tz})
		be := mock.New(ds)
		debug.FreeOSMemory() // return generator scratch memory before serving
		slog.Info("mock dataset ready", "size", *mockSize, "plays", len(ds.Plays), "tracks", len(ds.Tracks),
			"artists", len(ds.Artists), "albums", len(ds.Albums), "timezone", ds.Timezone, "took", time.Since(start).Round(time.Millisecond))
		opts.Mode, opts.Backend = "mock", be
		opts.Extra = map[string]http.Handler{"GET " + mock.ImagePath + "{kind}/{file}": be.ImageHandler(),
			"/_mock/scenario": be.ScenarioHandler()}
		wrap = be.Wrap
	}
	// MCP server (M9): the same Backend behind API-token auth (D16, D28).
	if opts.Extra == nil {
		opts.Extra = map[string]http.Handler{}
	}
	for pattern, h := range mcp.Routes(mcp.Options{Backend: opts.Backend, Auth: authSvc, Version: version, Mode: opts.Mode}) {
		opts.Extra[pattern] = h
	}
	if opts.UI == nil {
		slog.Warn("web UI not embedded; serving a placeholder at /")
	}

	idle := &idleRelease{quiet: 10 * time.Second}
	go idle.run(ctx)
	srv := &http.Server{
		Addr:              net.JoinHostPort(cfg.Host, strconv.Itoa(cfg.Port)),
		Handler:           idle.wrap(wrap(api.NewHandler(opts))),
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      60 * time.Second,
		IdleTimeout:       120 * time.Second,
	}
	errc := make(chan error, 1)
	go func() {
		slog.Info("listening", "addr", srv.Addr, "mode", opts.Mode, "version", version, "auth_mode", cfg.AuthMode)
		errc <- srv.ListenAndServe()
	}()
	select {
	case err := <-errc:
		return err
	case <-ctx.Done():
	}
	slog.Info("shutting down")
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}

// localZoneName returns the IANA name of the machine's timezone (TZ, or
// the /etc/localtime symlink), falling back to UTC.
func localZoneName() string {
	if tz := os.Getenv("TZ"); tz != "" {
		if _, err := time.LoadLocation(tz); err == nil {
			return tz
		}
	}
	if target, err := os.Readlink("/etc/localtime"); err == nil {
		if _, name, ok := strings.Cut(target, "zoneinfo/"); ok {
			if _, err := time.LoadLocation(name); err == nil {
				return name
			}
		}
	}
	return "UTC"
}

// announceSetupToken prints the first-run setup token as a banner on
// stderr (so it stands out in `docker logs`) and as a structured log line.
func announceSetupToken(token string) {
	fmt.Fprintf(os.Stderr, "\n"+
		"  ================================================================\n"+
		"   First-run setup: open the web UI and enter this setup token\n"+
		"   to set the owner password:\n\n"+
		"       %s\n\n"+
		"   It changes every time the server starts until setup is done.\n"+
		"  ================================================================\n\n", token)
	slog.Warn("first-run setup pending: enter the setup token shown above in the web UI", "setup_token", token)
}

// resetPassword removes the owner password and signs out every session, so
// the next visitor to the UI sets a new one (password mode recovery).
func resetPassword() error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	database, err := db.Open(context.Background(), cfg.DataDir)
	if err != nil {
		return err
	}
	defer database.Close()
	if err := auth.New(auth.Options{DB: database}).ResetPassword(context.Background()); err != nil {
		return err
	}
	fmt.Println("Owner password removed and all sessions signed out. Open the UI to set a new password;\n" +
		"it asks for the setup token the running server prints to its log (restart the server if you can't find it).")
	return nil
}

// importCmd imports export files synchronously (D23). It opens the same
// database as the server; WAL and the busy timeout let both write, and
// re-importing anything is a no-op.
func importCmd(paths []string) error {
	if len(paths) == 0 {
		return errors.New("usage: spotussi import <my_spotify_data.zip | Streaming_History_Audio_*.json | dir>...")
	}
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	database, err := db.Open(ctx, cfg.DataDir)
	if err != nil {
		return err
	}
	defer database.Close()
	failed := 0
	for _, p := range paths {
		fmt.Printf("Importing %s into %s\n", p, database.Path)
		start, last := time.Now(), time.Now()
		_, st, err := importer.ImportPath(ctx, database, p, func(st importer.Stats) {
			if time.Since(last) < time.Second {
				return
			}
			last = time.Now()
			pct := ""
			if st.BytesTotal > 0 {
				pct = fmt.Sprintf(" (%d%%)", st.BytesDone*100/st.BytesTotal)
			}
			fmt.Printf("  file %d/%d%s: %d rows read, %d new, %d already imported\n",
				min(st.FilesDone+1, st.Files), st.Files, pct, st.RowsDone, st.RowsNew, st.RowsDuplicate)
		})
		took := time.Since(start).Round(10 * time.Millisecond)
		if err != nil {
			failed++
			fmt.Printf("  failed after %s: %v\n  (%d rows read, %d new, %d already imported; imported rows are kept)\n",
				took, err, st.RowsDone, st.RowsNew, st.RowsDuplicate)
			if ctx.Err() != nil {
				break
			}
			continue
		}
		fmt.Printf("  done in %s: %d files, %d rows read, %d new, %d already imported\n",
			took, st.Files, st.RowsDone, st.RowsNew, st.RowsDuplicate)
		if st.RowsNew > 0 {
			var parts []string
			for _, ct := range []string{importer.ContentTrack, importer.ContentEpisode, importer.ContentAudiobook, importer.ContentUnknown} {
				if n := st.NewByType[ct]; n > 0 {
					parts = append(parts, fmt.Sprintf("%s %d", ct, n))
				}
			}
			fmt.Printf("  new rows by type: %s\n", strings.Join(parts, ", "))
		}
	}
	if failed > 0 {
		return fmt.Errorf("%d of %d imports failed", failed, len(paths))
	}
	return nil
}

// mcpCmd runs the stdio-to-HTTP MCP proxy. stdout carries the protocol,
// so everything else goes to stderr.
func mcpCmd(args []string) error {
	fs := flag.NewFlagSet("mcp", flag.ContinueOnError)
	url := fs.String("url", os.Getenv("SPOTUSSI_URL"), "spotussi base URL, e.g. https://stats.example.com (env SPOTUSSI_URL)")
	token := fs.String("token", "", "API token (default: env SPOTUSSI_TOKEN; prefer the env var, flags show up in ps)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *token == "" {
		*token = os.Getenv("SPOTUSSI_TOKEN")
	}
	if *url == "" || *token == "" {
		return errors.New("usage: spotussi mcp --url https://stats.example.com [--token spt_...] (or set SPOTUSSI_URL and SPOTUSSI_TOKEN)")
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	return mcp.RunProxy(ctx, mcp.ProxyOptions{URL: *url, Token: *token, In: os.Stdin, Out: os.Stdout, Log: os.Stderr})
}

// health is used by the container HEALTHCHECK (distroless has no curl).
func health() error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	client := http.Client{Timeout: 3 * time.Second}
	resp, err := client.Get(fmt.Sprintf("http://127.0.0.1:%d/api/v1/health", cfg.Port))
	if err != nil {
		return err
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("health: status %d", resp.StatusCode)
	}
	return nil
}
