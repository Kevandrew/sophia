package service

func (s *Service) TrustCheckStatusCR(id int) (*TrustCheckStatusReport, error) {
	return s.trustDomainService().trustCheckStatusCR(id)
}

func (s *Service) RunTrustChecksCR(id int) (*TrustCheckRunReport, error) {
	return s.trustDomainService().runTrustChecksCR(id)
}
