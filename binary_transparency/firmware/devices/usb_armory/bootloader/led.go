//+build armory

package main

import (
	"fmt"
	"time"

	usbarmory "github.com/f-secure-foundry/tamago/board/f-secure/usbarmory/mark-two"
)

// blinkOnOff blinks the specified LED on and then off.
func blinkOnOff(led string, on, off time.Duration) {
	usbarmory.LED(led, true)
	<-time.After(on)
	usbarmory.LED(led, false)
	<-time.After(off)
}

// haltAndCatchFire signals for help and never returns.
func haltAndCatchFire(msg string, code int) {
	fmt.Println(msg)

	on, off := 200*time.Millisecond, 300*time.Millisecond
	space := 300*time.Millisecond

	for {
		usbarmory.LED("blue", true)
		usbarmory.LED("white", true)

		<-time.After(space)
		usbarmory.LED("blue", false)
		<-time.After(space)

		for i := 0; i < code; i++ {
			blinkOnOff("blue", on, off)
		}
		usbarmory.LED("blue", false)
		usbarmory.LED("white", false)
		<-time.After(2*space)
	}
}

