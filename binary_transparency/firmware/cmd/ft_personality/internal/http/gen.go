package http

//go:generate mockgen -write_package_comment=false -self_package github.com/google/trillian-examples/binary_transparency/firmware/cmd/ft_personality/internal/http -package http -destination mock_trillian.go github.com/google/trillian-examples/binary_transparency/firmware/cmd/ft_personality/internal/http Trillian
