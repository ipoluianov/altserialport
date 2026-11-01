package forms

import (
	"context"
	"errors"
	"os/exec"

	"github.com/ipoluianov/nui/ui"
)

// popupOpen tells whether a menu or a list is open over the form of the
// widget: the keys are its then, not the widget's
func popupOpen(w *ui.Widget) bool {
	f := w.Form()
	return f != nil && f.TopPopupWidget() != nil
}

type ctxT = context.Context

// ctxErr tells whether the work was cancelled
func ctxErr(err error) bool {
	return errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded)
}

// startProcess starts the program again, for another window
func startProcess(exe string) error {
	cmd := exec.Command(exe)
	if err := cmd.Start(); err != nil {
		return err
	}
	go cmd.Wait()
	return nil
}
