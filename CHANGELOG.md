# Changelog

All notable changes to this project are documented here.
The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/).

## [Unreleased]

## [0.1.0] - 2026-11-20

### Added
- TLS client and server modes, selected by config.
- Per-connection IDs and per-direction byte counts in the logs.
- Startup checks for the remote TCP port and TLS handshake (log only).
- Config validation with clear error messages.
- Half-close handling and a drain timeout, so one finished direction does not hang the other.
- Minimum TLS 1.2 on both sides.