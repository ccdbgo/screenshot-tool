package main

import (
	_ "embed"

	"fyne.io/fyne/v2"
)

//go:embed assets/yijie.png
var _iconPngBytes []byte

var resourceIconPng fyne.Resource = &fyne.StaticResource{
	StaticName:    "yijie.png",
	StaticContent: _iconPngBytes,
}
