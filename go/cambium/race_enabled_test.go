// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 signalbreak-labs

//go:build race

package cambium_test

// raceDetectorEnabled reports whether the test binary runs under -race, which
// slows the time-bounded scaling tests several fold.
const raceDetectorEnabled = true
