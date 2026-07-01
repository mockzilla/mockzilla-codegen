package main

import "errors"

var (
	errNoMode       = errors.New("profile has no mode line")
	errBadLine      = errors.New("malformed profile line")
	errNoModule     = errors.New("go.mod has no module line")
	errBadPattern   = errors.New("bad ignore pattern")
	errBelowMinimum = errors.New("coverage below minimum")
)
