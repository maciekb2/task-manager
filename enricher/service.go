package main

type EnrichRequest struct {
	TaskDescription string `json:"task_description"`
	Priority        int32  `json:"priority"`
	URL             string `json:"url"`
	Method          string `json:"method"`
}

type EnrichResponse struct {
	Category string `json:"category"`
	Score    int32  `json:"score"`
}

type EnricherService struct{}

func NewEnricherService() *EnricherService {
	return &EnricherService{}
}

func (s *EnricherService) Enrich(req EnrichRequest) EnrichResponse {
	category := "low"
	if req.Priority >= 2 {
		category = "high"
	} else if req.Priority == 1 {
		category = "medium"
	}

	score := int32(len(req.URL)) + req.Priority*100
	return EnrichResponse{Category: category, Score: score}
}
