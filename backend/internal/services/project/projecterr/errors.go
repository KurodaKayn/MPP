package projecterr

import "errors"

var ErrInvalidProject = errors.New("invalid project")
var ErrInvalidProjectCollaborator = errors.New("invalid project collaborator")
var ErrInvalidProjectComment = errors.New("invalid project comment")
var ErrInvalidProjectShareLink = errors.New("invalid project share link")
var ErrInvalidProjectVersion = errors.New("invalid project version")
