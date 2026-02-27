package service

type taskLifecycleDomain struct {
	svc *Service
}

func newTaskLifecycleDomain(svc *Service) *taskLifecycleDomain {
	domain := &taskLifecycleDomain{}
	domain.bind(svc)
	return domain
}

func (d *taskLifecycleDomain) bind(svc *Service) {
	d.svc = svc
}

func (s *Service) taskLifecycleDomainService() *taskLifecycleDomain {
	if s == nil {
		return newTaskLifecycleDomain(nil)
	}
	if s.taskLifecycleSvc == nil {
		s.taskLifecycleSvc = newTaskLifecycleDomain(s)
	} else {
		s.taskLifecycleSvc.bind(s)
	}
	return s.taskLifecycleSvc
}
