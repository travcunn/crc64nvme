// Copyright 2026 Travis Cunningham. Licensed under the Apache License, Version 2.0.

//go:build purego || (!amd64 && !arm64)

package crc64nvme

// availableTiers lists the tiers this binary and CPU can run, generic first.
var availableTiers = []tier{tierGeneric}
