package blog

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func TestValidateArticle(t *testing.T) {
	tests := []struct {
		name  string
		title string
		body  string
		want  string
		err   error
	}{
		{name: "trim title", title: " \tСтатья\n", body: " \nТекст\n ", want: "Статья"},
		{name: "unicode boundaries", title: strings.Repeat("я", 200), body: strings.Repeat("я", 50000), want: strings.Repeat("я", 200)},
		{name: "empty title", body: "Text", err: ErrInvalidTitle},
		{name: "blank title", title: "\t\n\u2003", body: "Text", err: ErrInvalidTitle},
		{name: "long title", title: strings.Repeat("я", 201), body: "Text", err: ErrInvalidTitle},
		{name: "empty body", title: "Title", err: ErrInvalidBody},
		{name: "blank body", title: "Title", body: "\t\n\u2003", err: ErrInvalidBody},
		{name: "long body", title: "Title", body: strings.Repeat("я", 50001), err: ErrInvalidBody},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			title, err := validateArticle(tt.title, tt.body)
			if !errors.Is(err, tt.err) || title != tt.want {
				t.Fatalf("got (%q, %v), want (%q, %v)", title, err, tt.want, tt.err)
			}
		})
	}
}

func TestServiceRejectsInvalidWritesBeforeRepository(t *testing.T) {
	service := &Service{}
	ctx := context.Background()
	for _, id := range []int64{0, -1} {
		if _, err := service.Create(ctx, id, "Title", "Text"); !errors.Is(err, ErrInvalidAuthorID) {
			t.Fatalf("Create author %d: %v", id, err)
		}
		if _, err := service.Update(ctx, id, 1, "Title", "Text"); !errors.Is(err, ErrInvalidArticleID) {
			t.Fatalf("Update article %d: %v", id, err)
		}
		if _, err := service.Update(ctx, 1, id, "Title", "Text"); !errors.Is(err, ErrInvalidAuthorID) {
			t.Fatalf("Update author %d: %v", id, err)
		}
		if err := service.Delete(ctx, id, 1); !errors.Is(err, ErrInvalidArticleID) {
			t.Fatalf("Delete article %d: %v", id, err)
		}
		if err := service.Delete(ctx, 1, id); !errors.Is(err, ErrInvalidAuthorID) {
			t.Fatalf("Delete author %d: %v", id, err)
		}
	}
	for _, input := range []struct {
		title string
		body  string
		err   error
	}{
		{title: " ", body: "Text", err: ErrInvalidTitle},
		{title: "Title", body: " ", err: ErrInvalidBody},
	} {
		if _, err := service.Create(ctx, 1, input.title, input.body); !errors.Is(err, input.err) {
			t.Fatalf("Create invalid content: %v", err)
		}
		if _, err := service.Update(ctx, 1, 1, input.title, input.body); !errors.Is(err, input.err) {
			t.Fatalf("Update invalid content: %v", err)
		}
	}
}
