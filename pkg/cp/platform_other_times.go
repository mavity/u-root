//go:build !wasip1

package cp

import (
	"fmt"
	"time"
)

func setFileTimesNoFollow(string, time.Time, time.Time) error {
	return fmt.Errorf("preserve symlink timestamps: %w", errPlatformUnsupported)
}
