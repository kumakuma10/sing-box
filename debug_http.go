package box

import (
	"net/http"
	"net/http/pprof"
	"runtime"
	"runtime/debug"

	"github.com/kumakuma10/sing-box/common/badjson"
	"github.com/kumakuma10/sing-box/common/humanize"
	"github.com/kumakuma10/sing-box/common/json"
	"github.com/kumakuma10/sing-box/log"
	"github.com/kumakuma10/sing-box/option"
	E "github.com/sagernet/sing/common/exceptions"

	"github.com/go-chi/chi/v5"
)

var debugHTTPServer *http.Server

func applyDebugListenOption(options option.DebugOptions) {
	if debugHTTPServer != nil {
		debugHTTPServer.Close()
		debugHTTPServer = nil
	}
	if options.Listen == "" {
		return
	}
	r := chi.NewMux()
	r.Route("/debug", func(r chi.Router) {
		r.Get("/gc", func(writer http.ResponseWriter, request *http.Request) {
			writer.WriteHeader(http.StatusNoContent)
			go debug.FreeOSMemory()
		})
		r.Get("/memory", func(writer http.ResponseWriter, request *http.Request) {
			var memStats runtime.MemStats
			runtime.ReadMemStats(&memStats)

			var memObject badjson.JSONObject
			memObject.Put("heap", humanize.MemoryBytes(memStats.HeapInuse))
			memObject.Put("stack", humanize.MemoryBytes(memStats.StackInuse))
			memObject.Put("idle", humanize.MemoryBytes(memStats.HeapIdle-memStats.HeapReleased))
			memObject.Put("goroutines", runtime.NumGoroutine())
			memObject.Put("rss", rusageMaxRSS())

			encoder := json.NewEncoder(writer)
			encoder.SetIndent("", "  ")
			encoder.Encode(memObject)
		})
		r.HandleFunc("/pprof", pprof.Index)
		r.HandleFunc("/pprof/*", pprof.Index)
		r.HandleFunc("/pprof/cmdline", pprof.Cmdline)
		r.HandleFunc("/pprof/profile", pprof.Profile)
		r.HandleFunc("/pprof/symbol", pprof.Symbol)
		r.HandleFunc("/pprof/trace", pprof.Trace)
	})
	debugHTTPServer = &http.Server{
		Addr:    options.Listen,
		Handler: r,
	}
	go func() {
		err := debugHTTPServer.ListenAndServe()
		if err != nil && !E.IsClosed(err) {
			log.Error(E.Cause(err, "serve debug HTTP server"))
		}
	}()
}
