package llmprovider

import (
	"crypto/rand"
	"fmt"
	"net/http"
	"runtime"
	"runtime/debug"
	"sync"
)

// Client identification (MADR 0012 §1.4): every request names mcplib and the
// consuming application honestly, and never a reference client.
const (
	defaultClientName = "mcplib"
	mcplibModulePath  = "github.com/maccavelli/mcplib"
	headerUserAgent   = "User-Agent"
	// Kilo keys the editor and the task on these headers.
	kiloEditorHeader = "X-KILOCODE-EDITORNAME"
	kiloTaskHeader   = "X-KiloCode-TaskId"
	develVersion     = "(devel)"
)

// WithClientInfo names the consuming application: it leads User-Agent and is
// Kilo's editor name. An empty name keeps "mcplib"; an empty version keeps the
// build's own.
func WithClientInfo(name, version string) ProviderOption {
	return func(c *ProviderConfig) {
		c.ClientName = name
		c.ClientVersion = version
	}
}

// WithSessionID sets the conversation id sent as x-opencode-session and Kilo's
// task id. Empty keeps a random id, fixed for the provider's lifetime.
func WithSessionID(id string) ProviderOption {
	return func(c *ProviderConfig) {
		c.SessionID = id
	}
}

// buildVersions reads the versions of mcplib and of the main module once.
var buildVersions = sync.OnceValues(func() (mcplib, main string) {
	mcplib, main = develVersion, develVersion
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return mcplib, main
	}
	if v := info.Main.Version; v != "" {
		main = v
	}
	if info.Main.Path == mcplibModulePath {
		return main, main
	}
	for _, dep := range info.Deps {
		if dep.Path == mcplibModulePath && dep.Version != "" {
			mcplib = dep.Version
		}
	}
	return mcplib, main
})

// clientIdentity is who a provider says it is, and the session it speaks in.
type clientIdentity struct {
	name, version, session string
}

// identityOf resolves cfg's identity: the default name is mcplib at its own
// version; a named application defaults to the main module's version; a missing
// session is a fresh random id.
func identityOf(cfg ProviderConfig) clientIdentity {
	mcplib, main := buildVersions()
	id := clientIdentity{name: cfg.ClientName, version: cfg.ClientVersion, session: cfg.SessionID}
	if id.name == "" {
		id.name = defaultClientName
	}
	if id.version == "" {
		id.version = main
		if id.name == defaultClientName {
			id.version = mcplib
		}
	}
	if id.session == "" {
		id.session = rand.Text()
	}
	return id
}

func (id clientIdentity) userAgent() string {
	mcplib, _ := buildVersions()
	return fmt.Sprintf("%s/%s (%s; %s) mcplib/%s", id.name, id.version, runtime.GOOS, runtime.GOARCH, mcplib)
}

// apply returns cfg carrying this identity, for a listing the provider makes.
func (id clientIdentity) apply(cfg ProviderConfig) ProviderConfig {
	cfg.ClientName, cfg.ClientVersion, cfg.SessionID = id.name, id.version, id.session
	return cfg
}

// options carries this identity to a provider built for a probe.
func (id clientIdentity) options() []ProviderOption {
	return []ProviderOption{WithClientInfo(id.name, id.version), WithSessionID(id.session)}
}

// setUserAgent names the client on req.
func (id clientIdentity) setUserAgent(req *http.Request) {
	req.Header.Set(headerUserAgent, id.userAgent())
}
