// Package selfupdate is the canonical CLI self-update implementation for
// mcplib consumers.
//
// It discovers GitHub Releases, selects exact raw-binary assets, verifies
// SHA-256 integrity, and replaces a running executable through an injected
// installer. The package does not import a CLI framework, UI toolkit, or
// service manager. Consumers bind flags, streams, and lifecycle adapters.
//
// Baseline verification proves release-asset integrity: HTTPS GitHub
// metadata, the GitHub asset digest when present, and the exact SHA256SUMS
// entry. It does not prove publisher signature authenticity. Additional
// Verifier implementations may be composed after that integrity check
// without changing discovery, staging, or installation APIs.
package selfupdate
