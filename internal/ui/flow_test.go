package ui

import (
	"bytes"
	"errors"
	"strings"
	"testing"
)

func flowSession(input string) (*Session, *bytes.Buffer) {
	var out bytes.Buffer
	return NewSession(strings.NewReader(input), &out, nil, Colors{}), &out
}

// TestFlowDrawing runs an operation from a page and checks the whole
// drawing (D-069).
func TestFlowDrawing(t *testing.T) {
	s, out := flowSession("1\n1\n\n0\n")
	page := func() Menu {
		return Menu{Heading: "Page", Items: []Item{{Label: "Add", Action: func() error {
			s.Step(1, 2, "Type")
			if _, err := s.Choose(Menu{Heading: "Type", Items: []Item{{Label: "A"}}}); err != nil {
				return err
			}
			s.Println("checking")
			s.Step(2, 2, "Port")
			if _, err := s.PromptDefault("Port", "443"); err != nil {
				return err
			}
			s.Done("added")
			s.Println("link") // after Done: no gutter
			return nil
		}}}}
	}
	if err := s.Run(page); err != nil {
		t.Fatal(err)
	}
	want := "┌─ Add\n" +
		"│\n◆ 1/2 Type\n" +
		"│   1. A\n" +
		"│   0. 取消\n" +
		"│   请输入选项 [0-1]: " +
		"│   checking\n" +
		"│\n◆ 2/2 Port [443]: " +
		"│\n└─ ✓ added\n" +
		"link\n"
	if got := out.String(); !strings.Contains(got, want) {
		t.Errorf("got:\n%s\nwant:\n%s", got, want)
	}
}

func TestFlowEndings(t *testing.T) {
	for name, tc := range map[string]struct {
		action func(s *Session) error
		want   string
	}{
		"canceled": {func(*Session) error { return nil }, "│\n└─ 已取消\n"},
		"failed":   {func(*Session) error { return errors.New("boom") }, "│\n└─ ✗ 未完成\n"},
		"ended":    {func(s *Session) error { s.End("note"); return nil }, "│\n└─ note\n"},
	} {
		s, out := flowSession("1\n0\n")
		action := tc.action
		s.Run(func() Menu { return Menu{Items: []Item{{Label: "Op", Action: func() error { return action(s) }}}} })
		if !strings.Contains(out.String(), "┌─ Op\n"+tc.want) {
			t.Errorf("%s:\n%s", name, out.String())
		}
	}
}

func TestFlowSubChoiceAndPages(t *testing.T) {
	// A choice with a heading and no pending step is a sub-step: "◇",
	// option 0 is "back". A page item opens no flow.
	s, out := flowSession("1\n0\n2\n0\n0\n")
	var opened bool
	s.Run(func() Menu {
		return Menu{Items: []Item{
			{Label: "Op", Action: func() error {
				_, err := s.Choose(Menu{Heading: "Pick", Status: []string{"hint"}, Items: []Item{{Label: "x"}}})
				return err
			}},
			{Label: "Sub page", Page: true, Action: func() error {
				opened = true
				return s.Run(func() Menu { return Menu{Heading: "Inner"} })
			}},
		}}
	})
	got := out.String()
	if !strings.Contains(got, "│\n│   ◇ Pick\n│   hint\n│   1. x\n│   0. 返回\n") {
		t.Errorf("sub-choice:\n%s", got)
	}
	if !opened || strings.Contains(got, "┌─ Sub page") || !strings.Contains(got, HeadingBar("Inner")) {
		t.Errorf("page:\n%s", got)
	}
}

func TestOutsideFlow(t *testing.T) {
	s, out := flowSession("")
	s.Step(1, 2, "ignored")
	s.Println("plain")
	s.Done("done")
	if got := out.String(); got != "plain\n✓ done\n" {
		t.Errorf("got %q", got)
	}
}
