package worker

// ValidationError represents an error validating a single proxy configuration.
type ValidationError struct {
	Tag   string
	Error string
}

func ValidationErrorFromPB(pb *PBValidationError) ValidationError {
	if pb == nil {
		return ValidationError{}
	}
	return ValidationError{
		Tag:   PBBytesToString(pb.GetTag()),
		Error: pb.GetError(),
	}
}

func ValidationErrorsFromPB(pbs []*PBValidationError) []ValidationError {
	if len(pbs) == 0 {
		return nil
	}
	out := make([]ValidationError, len(pbs))
	for i, pb := range pbs {
		out[i] = ValidationErrorFromPB(pb)
	}
	return out
}

func (v ValidationError) ToPB() *PBValidationError {
	return &PBValidationError{
		Tag:   pbB(v.Tag),
		Error: v.Error,
	}
}

func ValidationErrorsToPB(vs []ValidationError) []*PBValidationError {
	if len(vs) == 0 {
		return nil
	}
	out := make([]*PBValidationError, len(vs))
	for i, v := range vs {
		out[i] = v.ToPB()
	}
	return out
}
