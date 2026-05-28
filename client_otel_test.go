/*
 * SPDX-FileCopyrightText: © 2017-2026 Istari Digital, Inc.
 * SPDX-License-Identifier: Apache-2.0
 */

package modusgraph

import (
	"testing"

	"google.golang.org/grpc"
)

func TestWithGRPCDialOption_RecordsOption(t *testing.T) {
	var opts clientOptions
	if len(opts.grpcDialOptions) != 0 {
		t.Fatalf("zero-value grpcDialOptions length = %d, want 0", len(opts.grpcDialOptions))
	}
	WithGRPCDialOption(grpc.EmptyDialOption{})(&opts)
	if len(opts.grpcDialOptions) != 1 {
		t.Fatalf("grpcDialOptions length = %d, want 1", len(opts.grpcDialOptions))
	}
}
