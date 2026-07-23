package widget

import (
	"context"
	"errors"
	"testing"
	"time"
)

type repositoryStub struct {
	createName  string
	create      Widget
	createErr   error
	getID       int64
	get         Widget
	getErr      error
	listOptions ListOptions
	list        []Widget
	listErr     error
	deleteID    int64
	deleteErr   error
}

func (stub *repositoryStub) Create(_ context.Context, name string) (Widget, error) {
	stub.createName = name

	return stub.create, stub.createErr
}

func (stub *repositoryStub) Get(_ context.Context, id int64) (Widget, error) {
	stub.getID = id

	return stub.get, stub.getErr
}

func (stub *repositoryStub) List(_ context.Context, options ListOptions) ([]Widget, error) {
	stub.listOptions = options

	return stub.list, stub.listErr
}

func (stub *repositoryStub) Delete(_ context.Context, id int64) error {
	stub.deleteID = id

	return stub.deleteErr
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

func TestServiceCreateRejectsNameLongerThanMaximum(t *testing.T) {
	t.Parallel()

	service := NewService(&repositoryStub{})

	_, err := service.Create(context.Background(), string(make([]rune, maximumNameLength+1)))
	if !errors.Is(err, ErrInvalidName) {
		t.Fatalf("Create() error = %v, want ErrInvalidName", err)
	}
}

func TestServicePreservesRepositoryErrors(t *testing.T) {
	t.Parallel()

	repositoryError := errors.New("repository failure")
	tests := []struct {
		name string
		call func(*Service) error
		stub *repositoryStub
	}{
		{
			name: "create",
			call: func(service *Service) error {
				_, err := service.Create(context.Background(), "example")

				return err
			},
			stub: &repositoryStub{createErr: repositoryError},
		},
		{
			name: "get",
			call: func(service *Service) error {
				_, err := service.Get(context.Background(), 1)

				return err
			},
			stub: &repositoryStub{getErr: repositoryError},
		},
		{
			name: "list",
			call: func(service *Service) error {
				_, err := service.List(context.Background(), ListOptions{Limit: 10})

				return err
			},
			stub: &repositoryStub{listErr: repositoryError},
		},
		{
			name: "delete",
			call: func(service *Service) error {
				return service.Delete(context.Background(), 1)
			},
			stub: &repositoryStub{deleteErr: repositoryError},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			if err := test.call(NewService(test.stub)); !errors.Is(err, repositoryError) {
				t.Fatalf("operation error = %v, want wrapped repository error", err)
			}
		})
	}
}

func TestServiceRejectsInvalidIdentifiers(t *testing.T) {
	t.Parallel()

	service := NewService(&repositoryStub{})

	if _, err := service.Get(context.Background(), 0); !errors.Is(err, ErrNotFound) {
		t.Fatalf("Get() error = %v, want ErrNotFound", err)
	}
	if err := service.Delete(context.Background(), -1); !errors.Is(err, ErrNotFound) {
		t.Fatalf("Delete() error = %v, want ErrNotFound", err)
	}
}

func TestServiceListAppliesDefaultLimit(t *testing.T) {
	t.Parallel()

	repository := &repositoryStub{list: []Widget{}}
	service := NewService(repository)

	if _, err := service.List(context.Background(), ListOptions{}); err != nil {
		t.Fatalf("List() unexpected error: %v", err)
	}
	if repository.listOptions.Limit != defaultListLimit+1 {
		t.Fatalf("repository limit = %d, want %d", repository.listOptions.Limit, defaultListLimit+1)
	}
}

func TestServiceListReturnsContinuationCursor(t *testing.T) {
	t.Parallel()

	createdAt := time.Unix(3, 0).UTC()
	repository := &repositoryStub{list: []Widget{
		{ID: 3, CreatedAt: createdAt},
		{ID: 2, CreatedAt: createdAt.Add(-time.Second)},
		{ID: 1, CreatedAt: createdAt.Add(-2 * time.Second)},
	}}

	page, err := NewService(repository).List(context.Background(), ListOptions{Limit: 2})
	if err != nil {
		t.Fatalf("List() unexpected error: %v", err)
	}
	if len(page.Items) != 2 {
		t.Fatalf("item count = %d, want 2", len(page.Items))
	}
	if page.NextCursor == nil || page.NextCursor.ID != 2 {
		t.Fatalf("next cursor = %+v, want widget 2", page.NextCursor)
	}
}

func TestServiceListRejectsInvalidPagination(t *testing.T) {
	t.Parallel()

	tests := []ListOptions{
		{Limit: -1},
		{Limit: maximumListLimit + 1},
		{Limit: 1, Cursor: &ListCursor{}},
		{Limit: 1, Cursor: &ListCursor{CreatedAt: time.Now(), ID: -1}},
	}

	for _, options := range tests {
		if _, err := NewService(&repositoryStub{}).List(context.Background(), options); !errors.Is(err, ErrInvalidPagination) {
			t.Errorf("List(%+v) error = %v, want ErrInvalidPagination", options, err)
		}
	}
}
