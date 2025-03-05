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

func EmptyDecorator() decor.Decorator {
	return decor.Any(func(s decor.Statistics) string {
		return ""
	})
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

func AddSpinner(progressBar *mpb.Progress, message string) *mpb.Bar {
	return progressBar.AddSpinner(
		1, mpb.BarFillerMiddleware(PositionSpinnerLeft),
		mpb.BarWidth(3),
		mpb.PrependDecorators(
			decor.OnComplete(EmptyDecorator(), emoji.SpinnerCheckMark),
			decor.OnAbort(EmptyDecorator(), emoji.SpinnerCrossMark),
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
