package widget

import (
	"context"
	"errors"
	"testing"
)

type repositoryStub struct {
	createName string
	create     Widget
	createErr  error
}

func (stub *repositoryStub) Create(_ context.Context, name string) (Widget, error) {
	stub.createName = name

	return stub.create, stub.createErr
}

func (stub *repositoryStub) Get(_ context.Context, _ int64) (Widget, error) {
	return Widget{}, ErrNotFound
}

func (stub *repositoryStub) List(_ context.Context, _ ListOptions) ([]Widget, error) {
	return []Widget{}, nil
}

func (stub *repositoryStub) Delete(_ context.Context, _ int64) error {
	return nil
}

func TestServiceCreateTrimsName(t *testing.T) {
	t.Parallel()

	repository := &repositoryStub{create: Widget{ID: 1, Name: "example"}}
	service := NewService(repository)

	result, err := service.Create(context.Background(), "  example  ")
	if err != nil {
		t.Fatalf("Create() unexpected error: %v", err)
	}
	if repository.createName != "example" {
		t.Fatalf("repository name = %q, want %q", repository.createName, "example")
	}
	if result.ID != 1 {
		t.Fatalf("result ID = %d, want 1", result.ID)
	}
}

func TestServiceCreateRejectsInvalidName(t *testing.T) {
	t.Parallel()

	service := NewService(&repositoryStub{})

	_, err := service.Create(context.Background(), "   ")
	if !errors.Is(err, ErrInvalidName) {
		t.Fatalf("Create() error = %v, want ErrInvalidName", err)
	}
}
