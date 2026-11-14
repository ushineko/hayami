package claude

// ForgetRefusals clears the refresh tokens the endpoint refused, so a test
// that was refused does not decide the next one's: the tests share a token.
func ForgetRefusals() { refused.Clear() }
