//go:build !linux && !android && !darwin && !windows && !wasip1

package touch

import (
	"errors"
	"os"
	"time"
)

func referenceAccessTime(os.FileInfo) (time.Time, error) {
	return time.Time{}, errors.ErrUnsupported
}
