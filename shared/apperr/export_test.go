package apperr

// RestoreErrorEncapsulation undoes DisableErrorEncapsulation, for a test that
// calls it to put things back. Only tests can.
func RestoreErrorEncapsulation() { encapsulationDisabled.Store(false) }
