package service

type mergeDomain struct {
	svc *Service
}

func newMergeDomain(svc *Service) *mergeDomain {
	domain := &mergeDomain{}
	domain.bind(svc)
	return domain
}

func (d *mergeDomain) bind(svc *Service) {
	d.svc = svc
}

func (s *Service) mergeDomainService() *mergeDomain {
	if s == nil {
		return newMergeDomain(nil)
	}
	if s.mergeSvc == nil {
		s.mergeSvc = newMergeDomain(s)
	} else {
		s.mergeSvc.bind(s)
	}
	return s.mergeSvc
}
