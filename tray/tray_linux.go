//go:build linux

package tray

// Linux: osxflow's own tray item and menu, for the reason in doc.go.
//
// The shape is soundmenu's, because it is the same problem: an item on the
// session bus, a menu this process draws itself, and one loop that owns
// both. osxflow's sni deliberately publishes no DBusMenu -- "the caller
// draws its own menu" -- so the menu here is a menu.Host popup rather than
// something the panel renders on our behalf. That is why this file is
// longer than its macOS counterpart, and why the menu looks like the rest
// of the desktop's menus rather than like a panel's.

import (
	"bytes"
	"errors"
	"image"
	"image/draw"
	"image/png"
	"log"
	"sync"

	"github.com/godbus/dbus/v5"

	"github.com/mpdroog/osxflow/menu"
	"github.com/mpdroog/osxflow/sni"
)

// menuWidth matches the other osxflow menus, in logical pixels.
const menuWidth = 260

// iconSizes are the pixmaps handed to the tray, which picks whichever is
// closest to the size it draws. The same set the other osxflow tools use.
var iconSizes = [...]int{22, 32, 44, 64}

// MenuItem is one row of the tray menu.
type MenuItem struct {
	mu    sync.Mutex
	title string

	sep     bool
	clicked chan struct{}
}

// SetTitle changes the row's label. It is picked up the next time the menu
// opens, which is the only time the label is read.
func (m *MenuItem) SetTitle(s string) {
	m.mu.Lock()
	m.title = s
	m.mu.Unlock()
}

// Clicked fires when the row is chosen.
func (m *MenuItem) Clicked() <-chan struct{} { return m.clicked }

func (m *MenuItem) label() string {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.title
}

// fire reports a click without blocking. A row clicked twice while the app
// is busy with the first is one click: the second would arrive after the
// dialog it opens has closed, which is not what anyone meant by it.
func (m *MenuItem) fire() {
	select {
	case m.clicked <- struct{}{}:
	default:
	}
}

// state is the tray, which there is one of. The mutex covers everything
// reachable from another goroutine: the app sets the icon and tooltip from
// its timer, while the loop below reads the rows to draw them.
var state struct {
	mu    sync.Mutex
	items []*MenuItem
	item  *sni.Item
	tip   sni.ToolTip

	quit     chan struct{}
	quitOnce sync.Once
}

// AddMenuItem appends a row.
func AddMenuItem(title, tooltip string) *MenuItem {
	m := &MenuItem{title: title, clicked: make(chan struct{}, 1)}
	state.mu.Lock()
	state.items = append(state.items, m)
	state.mu.Unlock()
	return m
}

// AddSeparator appends a divider.
func AddSeparator() {
	state.mu.Lock()
	state.items = append(state.items, &MenuItem{sep: true})
	state.mu.Unlock()
}

// SetIcon replaces the icon with the given PNG.
func SetIcon(data []byte) {
	pix, err := pixmaps(data)
	if err != nil {
		log.Printf("tray: %v", err)
		return
	}
	state.mu.Lock()
	it := state.item
	state.mu.Unlock()
	if it == nil {
		// Before Run got as far as exporting. Nothing is on screen yet,
		// and the app sets the icon again for every state it enters.
		return
	}
	if err := it.SetIcon(pix); err != nil {
		log.Printf("tray: setting the icon: %v", err)
	}
}

// SetTemplateIcon replaces the icon. The template half is macOS's -- it
// asks for the glyph to be drawn as a monochrome mask -- and no tray here
// honours one, so the regular image is used.
func SetTemplateIcon(template, regular []byte) { SetIcon(regular) }

// SetTitle is a no-op on Linux, and deliberately so.
//
// StatusNotifierItem's Title is fixed when the item is exported, and the
// panel here does not draw it in any case: menubar shows an item's icon,
// falling back to the first letter of its title only when it was sent no
// pixels at all. The elapsed time the app passes here reaches the screen
// through SetTooltip, which is called with the same value.
func SetTitle(string) {}

// SetTooltip sets the hover text.
func SetTooltip(s string) {
	state.mu.Lock()
	state.tip.Title = "EZHours"
	state.tip.Text = s
	tip, it := state.tip, state.item
	state.mu.Unlock()
	if it == nil {
		return
	}
	if err := it.SetToolTip(tip); err != nil {
		log.Printf("tray: setting the tooltip: %v", err)
	}
}

// SetOnTapped is a no-op on Linux: a click opens the menu, which is what
// every tray here expects and what the menu exists for.
func SetOnTapped(func()) {}

// Quit takes the icon down and lets Run return.
func Quit() {
	state.quitOnce.Do(func() {
		if state.quit != nil {
			close(state.quit)
		}
	})
}

// Run shows the icon and blocks until Quit.
func Run(onReady, onExit func()) {
	state.quit = make(chan struct{})
	if err := run(onReady); err != nil {
		log.Printf("tray: %v", err)
	}
	onExit()
}

func run(onReady func()) error {
	conn, err := dbus.SessionBus()
	if err != nil {
		return err
	}

	// Native on whichever display server the session runs: a layer
	// surface straight on the compositor under Wayland, an X window on X.
	// The host owns the connection it dials, and Release closes it.
	host, err := menu.Dial(0, menuWidth)
	if err != nil {
		return err
	}
	defer func() {
		if err := host.Release(); err != nil {
			log.Printf("tray: releasing the menu host: %v", err)
		}
	}()

	// Only one osxflow menu open at a time, the same as the others.
	closeMenus, err := host.Exclusive(conn)
	if err != nil {
		return err
	}

	item, err := sni.Export(conn, &sni.Options{
		ID:       "ezhours",
		Title:    "EZHours",
		Category: "ApplicationStatus",
	})
	if err != nil {
		return err
	}
	defer func() {
		if err := item.Close(); err != nil {
			log.Printf("tray: closing the item: %v", err)
		}
	}()

	state.mu.Lock()
	state.item = item
	state.mu.Unlock()

	// Now the item exists, so the icon and tooltip the app sets have
	// somewhere to go.
	onReady()

	events, gone := host.Events(), host.Gone()
	for {
		select {
		case ev := <-events:
			if err := host.Handle(ev); err != nil {
				log.Printf("tray: %v", err)
			}

		case err := <-gone:
			return err

		case c := <-item.Clicks():
			switch c.Kind {
			case sni.Activate, sni.ContextMenu:
				openMenu(host, c)
			}

		case <-closeMenus:
			// Another osxflow menu opened.
			host.Close()

		case err := <-item.Registrations():
			switch {
			case errors.Is(err, sni.ErrNoWatcher):
				log.Printf("tray: %v; the icon appears when a tray starts", err)
			case err != nil:
				log.Printf("tray: %v", err)
			}

		case <-state.quit:
			return nil
		}
	}
}

// openMenu draws the menu under the icon, or puts it away when it is
// already showing.
//
// The second click has to close it. The menu takes the pointer grab, so
// the click that dismisses it never reaches the icon underneath -- it
// arrives here as another Activate, and reopening on it made the menu look
// like it could not be dismissed by the thing that opened it.
func openMenu(host *menu.Host, c sni.Click) {
	if host.IsOpen() {
		host.Close()
		return
	}
	// Where the tray says the icon is, for placing the menu when the
	// compositor publishes no work area -- which labwc does not, having no
	// way to describe a layer-shell panel in EWMH terms.
	host.AnchorY = c.Y

	state.mu.Lock()
	items := make([]*MenuItem, len(state.items))
	copy(items, state.items)
	state.mu.Unlock()

	rows := make([]menu.Row, 0, len(items))
	for _, m := range items {
		if m.sep {
			rows = append(rows, menu.Row{Kind: menu.Separator})
			continue
		}
		rows = append(rows, menu.Row{Kind: menu.Action, Label: m.label(), Click: m.fire})
	}

	// The x the tray reports is in the panel's own coordinates and is not
	// always usable; MenuX turns it into where the menu should sit, and
	// the pointer is the fallback when the tray sent nothing.
	x := c.X
	if x == 0 {
		if px, err := host.PointerX(); err == nil {
			x = px
		}
	}
	centre, err := host.MenuX(x)
	if err != nil {
		log.Printf("tray: placing the menu: %v", err)
		centre = x
	}
	if err := host.Open(centre, rows); err != nil {
		log.Printf("tray: opening the menu: %v", err)
	}
}

// pixmaps turns the app's PNG into the sizes the tray chooses between.
func pixmaps(data []byte) ([]sni.Pixmap, error) {
	src, err := png.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	out := make([]sni.Pixmap, 0, len(iconSizes))
	for _, size := range iconSizes {
		out = append(out, sni.FromImage(scaleTo(src, size)))
	}
	return out, nil
}

// scaleTo redraws src at size x size. The icons this is given are already
// square, so nothing needs to be letterboxed.
func scaleTo(src image.Image, size int) *image.RGBA {
	dst := image.NewRGBA(image.Rect(0, 0, size, size))
	if b := src.Bounds(); b.Dx() == size && b.Dy() == size {
		draw.Draw(dst, dst.Bounds(), src, b.Min, draw.Src)
		return dst
	}
	nearest(dst, src)
	return dst
}

// nearest is enough here: the source icons are drawn large (64px) and the
// sizes asked for are close to it, so the difference against a filtered
// scale is not visible at tray size -- and this needs no dependency.
func nearest(dst *image.RGBA, src image.Image) {
	b := src.Bounds()
	w, h := dst.Bounds().Dx(), dst.Bounds().Dy()
	for y := range h {
		sy := b.Min.Y + y*b.Dy()/h
		for x := range w {
			sx := b.Min.X + x*b.Dx()/w
			dst.Set(x, y, src.At(sx, sy))
		}
	}
}
