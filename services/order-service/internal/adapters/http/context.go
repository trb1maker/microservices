package http

import (
	"net/http"

	"github.com/trb1maker/microservices/services/order-service/internal/domain"

	pkgmiddleware "github.com/trb1maker/microservices/pkg/middleware"
)

type callerIdentity struct {
	userID    domain.UserID
	isService bool
}

func callerFromRequest(r *http.Request) (callerIdentity, error) {
	if userID, ok := pkgmiddleware.UserIDFromContext(r.Context()); ok {
		return callerIdentity{userID: domain.UserID(userID)}, nil
	}

	if _, ok := pkgmiddleware.ServiceNameFromContext(r.Context()); ok {
		return callerIdentity{isService: true}, nil
	}

	return callerIdentity{}, errUnauthorized
}

func userIDFromRequest(r *http.Request) (domain.UserID, error) {
	caller, err := callerFromRequest(r)
	if err != nil {
		return domain.UserID{}, err
	}

	if caller.isService {
		return domain.UserID{}, errUnauthorized
	}

	return caller.userID, nil
}
