// Code generated by "stringer -type=Level -linecomment"; DO NOT EDIT.

package keyboard

import "strconv"

func _() {
	// An "invalid array index" compiler error signifies that the constant values have changed.
	// Re-run the stringer command to generate them again.
	var x [1]struct{}
	_ = x[OFF-0]
	_ = x[LOW-1]
	_ = x[MEDIUM-2]
	_ = x[HIGH-3]
}

const _Level_name = "OffLowMediumHigh"

var _Level_index = [...]uint8{0, 3, 6, 12, 16}

func (i Level) String() string {
	if i >= Level(len(_Level_index)-1) {
		return "Level(" + strconv.FormatInt(int64(i), 10) + ")"
	}
	return _Level_name[_Level_index[i]:_Level_index[i+1]]
}
