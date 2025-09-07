package main

import (
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/widget"
)

const animationDuration = 250 * time.Millisecond

// AnimateShow smoothly reveals the object over animationDuration.
func AnimateShow(obj fyne.CanvasObject) {
	final := obj.MinSize()
	obj.Resize(fyne.NewSize(0, 0))
	obj.Show()
	anim := canvas.NewSizeAnimation(fyne.NewSize(0, 0), final, animationDuration, func(s fyne.Size) {
		obj.Resize(s)
	})
	anim.Start()
}

// AnimateHide smoothly hides the object over animationDuration.
func AnimateHide(obj fyne.CanvasObject) {
	start := obj.Size()
	anim := canvas.NewSizeAnimation(start, fyne.NewSize(0, 0), animationDuration, func(s fyne.Size) {
		obj.Resize(s)
	})
	anim.SetCompletionCallback(func() {
		obj.Hide()
		obj.Resize(start)
	})
	anim.Start()
}

// AnimatedButton wraps widget.Button with animated visibility.
// It promotes all button methods and overrides Show/Hide.
type AnimatedButton struct {
	*widget.Button
}

// NewAnimatedButton creates a new button with animated visibility.
func NewAnimatedButton(label string, tapped func()) *AnimatedButton {
	btn := widget.NewButton(label, tapped)
	return &AnimatedButton{Button: btn}
}

// NewAnimatedButtonWithIcon creates a new icon button with animated visibility.
func NewAnimatedButtonWithIcon(label string, icon fyne.Resource, tapped func()) *AnimatedButton {
	btn := widget.NewButtonWithIcon(label, icon, tapped)
	return &AnimatedButton{Button: btn}
}

// Show reveals the button with animation.
func (b *AnimatedButton) Show() {
	AnimateShow(b.Button)
}

// Hide conceals the button with animation.
func (b *AnimatedButton) Hide() {
	AnimateHide(b.Button)
}
