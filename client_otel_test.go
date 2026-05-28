/*
 * SPDX-FileCopyrightText: © 2017-2026 Istari Digital, Inc.
 * SPDX-License-Identifier: Apache-2.0
 */

package modusgraph

import "testing"

func TestWithGRPCDialOption_RecordsOption(t *testing.T) {
	var opts clientOptions
	WithGRPCDialOption(nil)(&opts)
	if len(opts.grpcDialOptions) != 1 {
		t.Fatalf("grpcDialOptions length = %d, want 1", len(opts.grpcDialOptions))
	}
}
