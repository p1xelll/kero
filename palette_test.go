package main

import (
	"testing"

	"github.com/egoist/mygo/i18n"
	"github.com/egoist/mygo/ui"

	"kero/locales"
)

// The palette finds commands by their keywords and English names, in a
// language whose titles have neither.
func TestPaletteKeywords(t *testing.T) {
	a := testApp(t)
	tt := ui.NewTester(a.view, 1100, 640)
	i18n.SetLocaleFunc(func() string { return "zh-Hans" })
	t.Cleanup(func() { i18n.SetLocaleFunc(nil) })
	a.openPalette()
	tt.Frame()
	for query, want := range map[string]string{
		"browser":  locales.In("zh-Hans").ActionNewBrowser(),
		"adblock":  locales.In("zh-Hans").PaletteSettingsPage(locales.In("zh-Hans").PageBrowser()),
		"maximize": locales.In("zh-Hans").ActionZoomPane(),
	} {
		a.pal.query = ""
		tt.Frame()
		tt.Type(query)
		tt.Frame()
		if !tt.HasText(want) {
			t.Errorf("%q finds no %q; texts %q", query, want, tt.Texts())
		}
	}
	// Keywords match by their starts, not as letters strewn through them.
	if keywords("bsr", actionWords["pane.browser"]) >= 0 {
		t.Error("keywords matched letters in order")
	}
}

// The wheel scrolls the palette's list away from the one chosen.
func TestPaletteScrolls(t *testing.T) {
	a := testApp(t)
	tt := ui.NewTester(a.view, 1100, 900)
	a.openPalette()
	tt.Frame()
	r, ok := tt.Find(a.paletteItems()[3].title)
	if !ok {
		t.Fatalf("no commands shown; texts %q", tt.Texts())
	}
	tt.Scroll(r.X+20, r.Y, 0, 300)
	tt.Frame()
	tt.Frame()
	if y := a.pal.scroll.Y; y < 100 {
		t.Errorf("the wheel scrolled the palette to %g, want 300", y)
	}
}

func TestCancelEndingConfirmation(t *testing.T) {
	a := testApp(t)
	ran := false
	a.confirm = &confirmation{
		open: true, ends: true, title: "End programs?", action: "End programs",
		run: func() { ran = true },
	}
	tt := ui.NewTester(a.view, 1100, 640)
	tt.Frame()
	tt.Key(0, ui.KeyEnter)
	tt.Frame()
	if a.confirm == nil || ran {
		t.Fatal("Return answered a confirmation that requires a click")
	}
	tt.Key(0, ui.KeyTab)
	tt.Frame()
	tt.Key(0, ui.KeyEnter)
	tt.Frame()
	if a.confirm == nil || ran {
		t.Fatal("Return answered a confirmation after moving focus")
	}
	if err := tt.Click(locales.CommonCancel()); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if a.confirm != nil || ran {
		t.Errorf("Cancel left confirmation = %+v, ran = %v, want nil and false", a.confirm, ran)
	}
	tt.Frame()
}
