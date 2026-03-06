package service

import (
	"sort"

	"sophia/internal/model"
)

type crReadModel struct {
	all              []model.CR
	byID             map[int]model.CR
	childrenByParent map[int][]model.CR
}

type CRReadModelView struct {
	readModel *crReadModel
}

func (s *Service) loadCRReadModel() (*crReadModel, error) {
	crs, err := s.store.ListCRs()
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
	}
	for _, cr := range crs {
		rm.byID[cr.ID] = cr
		if cr.ParentCRID > 0 {
			rm.childrenByParent[cr.ParentCRID] = append(rm.childrenByParent[cr.ParentCRID], cr)
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
