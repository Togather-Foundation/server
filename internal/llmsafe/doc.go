// Package llmsafe provides shared LLM prompt-injection defense: boundary tagging
// with cryptographic nonces and HTML sanitization.
//
// The defense-in-depth strategy combines nonce-guarded boundary markers that
// instruct LLMs to treat enclosed content as DATA not instructions, with
// stripping of script, style, and comment tags that carry injection risk.
//
// See specs/004-agentic-maintainer/spec-phase1.md for the full design.
package llmsafe
