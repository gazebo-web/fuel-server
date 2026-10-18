// Command aibuilder is the AI Builder's in-app agent (design §3.2.2): the
// chat loop over the user's own LLM key, calling the aitools tools
// in-process as the signed-in user. It is a second binary in the
// fuel-server module, run beside fuel-server on the same host: it shares
// the database and the /fuel volume and serves the web app's chat.
//
// Configuration: fuel-server's environment (database, AUTH0_RSA256_PUBLIC_KEY,
// IGN_FUEL_RESOURCE_DIR, AI_TOOLS_*), plus
//
//	AI_BUILDER_PORT                 listen port (8010)
//	AI_BUILDER_KEY_ENCRYPTION_KEY   32 bytes, hex: encrypts users' LLM keys. Only this binary has it.
//	AI_BUILDER_ALLOWED_ORIGIN       the web app's origin for CORS (http://localhost:3001)
//	AI_BUILDER_EFFORT               reasoning effort on models that support it (high)
package main

import (
	"encoding/hex"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/joho/godotenv"

	"github.com/gazebo-web/fuel-server/aibuilder"
	"github.com/gazebo-web/fuel-server/aibuilder/llm"
	"github.com/gazebo-web/fuel-server/aibuilder/llm/providers"
	"github.com/gazebo-web/fuel-server/aitools"
	"github.com/gazebo-web/fuel-server/bootstrap"
	"github.com/gazebo-web/fuel-server/globals"
)

func env(name, def string) string {
	if v := os.Getenv(name); v != "" {
		return v
	}
	return def
}

func main() {
	_ = godotenv.Load()
	key, err := hex.DecodeString(os.Getenv("AI_BUILDER_KEY_ENCRYPTION_KEY"))
	if err != nil || len(key) != 32 {
		log.Fatal("AI_BUILDER_KEY_ENCRYPTION_KEY must be 32 bytes, hex-encoded")
	}
	enc, err := aibuilder.NewEncryptor(key)
	if err != nil {
		log.Fatal(err)
	}
	bootstrap.Shared("aibuilder")

	reg := llm.NewRegistry()
	reg.Register("anthropic", func(apiKey, endpoint string) (llm.Provider, error) {
		return providers.NewAnthropic(apiKey, endpoint)
	})
	reg.Register("openai", func(apiKey, endpoint string) (llm.Provider, error) {
		return providers.NewOpenAI(apiKey, endpoint)
	})
	reg.Register("gemini", func(apiKey, endpoint string) (llm.Provider, error) {
		return providers.NewGemini(apiKey, endpoint)
	})
	reg.Register("ollama", func(apiKey, endpoint string) (llm.Provider, error) {
		return providers.NewOllama(apiKey, endpoint)
	})

	toolCfg := aitools.ConfigFromEnv()
	toolCfg.SweepSizes = false // fuel-server sweeps
	db := globals.Server.Db
	srv, err := aibuilder.NewServer(db, aibuilder.Config{
		Encryptor:     enc,
		Providers:     reg,
		Tools:         aitools.NewRegistry(toolCfg),
		Auth:          aibuilder.GzAuth(db, globals.Server),
		Effort:        env("AI_BUILDER_EFFORT", "high"),
		AllowedOrigin: env("AI_BUILDER_ALLOWED_ORIGIN", "http://localhost:3001"),
	})
	if err != nil {
		log.Fatal(err)
	}
	addr := ":" + env("AI_BUILDER_PORT", "8010")
	log.Printf("aibuilder: listening on %s", addr)
	hs := &http.Server{Addr: addr, Handler: srv.Handler(), ReadHeaderTimeout: 10 * time.Second}
	log.Fatal(hs.ListenAndServe())
}
