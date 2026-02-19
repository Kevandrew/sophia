package service

func (s *Service) PreviewCRPatch(selector string, patchBytes []byte, force bool) (*CRPatchApplyResult, error) {
	return s.ApplyCRPatch(selector, patchBytes, force, true)
}
