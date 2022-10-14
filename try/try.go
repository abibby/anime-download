package try

type tryError error

func Try[T any](v T, err error) T {
	if err != nil {
		panic(tryError(err))
	}
	return v
}

func Try0(err error) {
	if err != nil {
		panic(tryError(err))
	}
}
func Try1[T any](v T, err error) T {
	if err != nil {
		panic(tryError(err))
	}
	return v
}

func Handle(cb func(err error)) {
	r := recover()
	tryErr, ok := r.(tryError)
	if !ok {
		return
	}
	err := error(tryErr)
	if err != nil {
		cb(err)
	}
}
