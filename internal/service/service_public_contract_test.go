package service

import "sophia/internal/model"

// Compile-time checks to guard public Service method signatures used by CLI wiring.
var (
	_ func(*Service, string, string) (*model.CR, error)                         = (*Service).AddCR
	_ func(*Service, int, string, bool) (*model.CR, error)                      = (*Service).SetCRBase
	_ func(*Service, int) (*model.CR, error)                                    = (*Service).RestackCR
	_ func(*Service, int) (*WhyView, error)                                     = (*Service).WhyCR
	_ func(*Service, int) (*CRStatusView, error)                                = (*Service).StatusCR
	_ func(*Service, int) (*ValidationReport, error)                            = (*Service).ValidateCR
	_ func(*Service, int, bool, string) (string, error)                         = (*Service).MergeCR
	_ func(*Service, int, MergeCROptions) (*MergeCRResult, error)               = (*Service).MergeCRWithOptions
	_ func(*Service, int) (*MergeStatusView, error)                             = (*Service).MergeStatusCR
	_ func(*Service, int) error                                                 = (*Service).AbortMergeCR
	_ func(*Service, int, bool, string) (string, []string, error)               = (*Service).ResumeMergeCR
	_ func(*Service, int, MergeCROptions) (*MergeCRResult, error)               = (*Service).ResumeMergeCRWithOptions
	_ func(*Service, string, string, AddCROptions) (*model.CR, []string, error) = (*Service).AddCRWithOptionsWithWarnings
	_ func(*Service, string, string, AddCROptions) (*AddCRResult, error)        = (*Service).AddCRWithOptions
)
