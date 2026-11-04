//go:build !unix

package readings

// TryLock always succeeds where flock is unavailable: the only caller is a
// section that does not run on these platforms (spec 085 R3).
func TryLock(string) (release func(), ok bool, err error) { return func() {}, true, nil }
