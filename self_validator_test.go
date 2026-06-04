/*
 * SPDX-FileCopyrightText: © 2017-2026 Istari Digital, Inc.
 * SPDX-License-Identifier: Apache-2.0
 */

package modusgraph

import (
	"context"
	"errors"
	"testing"
)

// recordingValidator counts StructCtx calls so tests can assert which path ran.
type recordingValidator struct{ calls int }

func (r *recordingValidator) StructCtx(_ context.Context, _ interface{}) error {
	r.calls++
	return nil
}

var errSelfValidated = errors.New("self-validated")

type selfValidatingEntity struct{ Name string }

func (s *selfValidatingEntity) ValidateWith(_ context.Context, _ StructValidator) error {
	return errSelfValidated
}

type plainEntity struct{ Name string }

func TestValidateRoutesToSelfValidator(t *testing.T) {
	rv := &recordingValidator{}
	c := client{options: clientOptions{validator: rv}}

	err := c.validateStruct(context.Background(), &selfValidatingEntity{Name: "x"})
	if !errors.Is(err, errSelfValidated) {
		t.Fatalf("expected the SelfValidator path, got %v", err)
	}
	if rv.calls != 0 {
		t.Fatalf("StructCtx must not run for a SelfValidator, got %d calls", rv.calls)
	}
}

func TestValidateFallsBackToStructCtx(t *testing.T) {
	rv := &recordingValidator{}
	c := client{options: clientOptions{validator: rv}}

	if err := c.validateStruct(context.Background(), &plainEntity{Name: "x"}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if rv.calls != 1 {
		t.Fatalf("expected StructCtx to run once, got %d", rv.calls)
	}
}

func TestValidateSelfValidatorInSlice(t *testing.T) {
	rv := &recordingValidator{}
	c := client{options: clientOptions{validator: rv}}

	err := c.validateStruct(context.Background(), []*selfValidatingEntity{{Name: "a"}})
	if !errors.Is(err, errSelfValidated) {
		t.Fatalf("expected the SelfValidator path for slice elements, got %v", err)
	}
}
