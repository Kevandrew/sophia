package service

import (
	"sort"

	"sophia/internal/model"
)

type crReadModel struct {
	all              []model.CR
	byID             map[int]model.CR
	childrenByParent map[int][]model.CR
	effectiveParents map[int]int
}

type CRReadModelView struct {
	readModel *crReadModel
}

func (s *Service) loadCRReadModel() (*crReadModel, error) {
	crs, err := s.activeLifecycleStoreProvider().ListCRs()
	if err != nil {
		return nil, err
	}
	return buildCRReadModel(crs), nil
}

func (s *Service) LoadCRReadModelForCLI() (*CRReadModelView, error) {
	readModel, err := s.loadCRReadModel()
	if err != nil {
		return nil, err
	}
	return &CRReadModelView{readModel: readModel}, nil
}

func buildCRReadModel(crs []model.CR) *crReadModel {
	rm := &crReadModel{
		all:              append([]model.CR(nil), crs...),
		byID:             make(map[int]model.CR, len(crs)),
		childrenByParent: make(map[int][]model.CR),
		effectiveParents: make(map[int]int, len(crs)),
	}
	rm.all = make([]model.CR, 0, len(crs))
	for _, cr := range crs {
		rm.effectiveParents[cr.ID] = effectiveParentCRID(cr, crs)
	}
	for _, cr := range crs {
		normalized := cr
		normalized.ParentCRID = rm.effectiveParentID(cr.ID)
		rm.byID[normalized.ID] = normalized
		rm.all = append(rm.all, normalized)
		parentID := normalized.ParentCRID
		if parentID > 0 {
			rm.childrenByParent[parentID] = append(rm.childrenByParent[parentID], normalized)
		}
	}
	for parentID := range rm.childrenByParent {
		children := rm.childrenByParent[parentID]
		sort.SliceStable(children, func(i, j int) bool {
			if children[i].ID == children[j].ID {
				return children[i].Title < children[j].Title
			}
			return children[i].ID < children[j].ID
		})
		rm.childrenByParent[parentID] = children
	}
	sort.SliceStable(rm.all, func(i, j int) bool {
		return rm.all[i].ID < rm.all[j].ID
	})
	return rm
}

func effectiveParentCRID(cr model.CR, all []model.CR) int {
	if expectedParentID := expectedParentCRIDFromBaseRef(cr.BaseRef, cr.ID, all); expectedParentID > 0 {
		return expectedParentID
	}
	return cr.ParentCRID
}

func (rm *crReadModel) effectiveParentID(crID int) int {
	if rm == nil {
		return 0
	}
	return rm.effectiveParents[crID]
}

func (rm *crReadModel) normalizeCR(cr model.CR) model.CR {
	if rm == nil {
		return cr
	}
	if normalized, ok := rm.byID[cr.ID]; ok {
		return normalized
	}
	cr.ParentCRID = rm.effectiveParentID(cr.ID)
	return cr
}

func (rm *crReadModel) crByID(id int) (model.CR, bool) {
	if rm == nil {
		return model.CR{}, false
	}
	cr, ok := rm.byID[id]
	return cr, ok
}

func (rm *crReadModel) childrenOf(parentID int) []model.CR {
	if rm == nil {
		return nil
	}
	return rm.childrenByParent[parentID]
}

func (v *CRReadModelView) AllCRsForCLI() []model.CR {
	if v == nil || v.readModel == nil {
		return nil
	}
	return append([]model.CR(nil), v.readModel.all...)
}

func (v *CRReadModelView) CRByIDForCLI(id int) (model.CR, bool) {
	if v == nil || v.readModel == nil {
		return model.CR{}, false
	}
	return v.readModel.crByID(id)
}
