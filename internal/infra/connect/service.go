package connect

import (
	"context"

	greetv1 "github.com/pj-hoakari/go-service-template/gen/greet/v1"
	"github.com/pj-hoakari/go-service-template/gen/greet/v1/greetv1connect"
	"github.com/pj-hoakari/go-service-template/internal/application"
)

// Service is the Connect transport implementation of GreetService.
type Service struct {
	greetv1connect.UnimplementedGreetServiceHandler
	greetService application.GreetUseCases
}

func NewService(greetService application.GreetUseCases) *Service {
	return &Service{
		UnimplementedGreetServiceHandler: greetv1connect.UnimplementedGreetServiceHandler{},
		greetService:                     greetService,
	}
}

func (s *Service) Greet(ctx context.Context, req *greetv1.GreetRequest) (*greetv1.GreetResponse, error) {
	greeting, err := s.greetService.Greet(ctx, application.GreetInput{
		Name: req.GetName(),
	})
	if err != nil {
		return nil, connectError(ctx, err)
	}

	return &greetv1.GreetResponse{
		Greeting: greeting.Message(),
	}, nil
}
