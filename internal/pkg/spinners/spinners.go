package spinners

import (
	"io"

	"github.com/vbauerster/mpb/v8"
	"github.com/vbauerster/mpb/v8/decor"

	"github.com/openshift/oc-mirror/v2/internal/pkg/emoji"
)

func PositionSpinnerLeft(original mpb.BarFiller) mpb.BarFiller {
	return mpb.SpinnerStyle("⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏", " ").PositionLeft().Build()
}

func BarFillerClearOnAbort() mpb.BarOption {
	return mpb.BarFillerMiddleware(func(base mpb.BarFiller) mpb.BarFiller {
		return mpb.BarFillerFunc(func(w io.Writer, st decor.Statistics) error {
			if st.Aborted {
				_, err := io.WriteString(w, "")
				return err
			}
			return base.Fill(w, st)
		})
	})
}

// statusDecorator renders a single success or failure mark.
//
// mpb v8.15.2 sets both Completed and Aborted on a successful bar's
// final PopCompletedMode frame (bar context cancel uses context.Canceled
// as the cause). Prefer Completed so a successful copy does not also
// show the failure mark.
func statusDecorator() decor.Decorator {
	return decor.Any(func(s decor.Statistics) string {
		if s.Completed {
			return emoji.SpinnerCheckMark
		}
		if s.Aborted {
			return emoji.SpinnerCrossMark
		}
		return ""
	})
}

func AddSpinner(progressBar *mpb.Progress, message string) *mpb.Bar {
	return progressBar.AddSpinner(
		1, mpb.BarFillerMiddleware(PositionSpinnerLeft),
		mpb.BarWidth(3),
		mpb.PrependDecorators(
			statusDecorator(),
		),
		mpb.AppendDecorators(
			decor.Name("("),
			decor.Elapsed(decor.ET_STYLE_GO),
			decor.Name(") "+message+" "),
		),
		mpb.BarFillerClearOnComplete(),
		BarFillerClearOnAbort(),
	)
}
