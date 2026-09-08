package rekognition

import (
	"context"
	"fmt"

	"github.com/blackbirdworks/gopherstack/pkgs/service"
)

func (h *Handler) userOps() map[string]service.JSONOpFunc {
	return map[string]service.JSONOpFunc{
		"CreateUser":         service.WrapOp(h.handleCreateUser),
		"DeleteUser":         service.WrapOp(h.handleDeleteUser),
		"ListUsers":          service.WrapOp(h.handleListUsers),
		"AssociateFaces":     service.WrapOp(h.handleAssociateFaces),
		"DisassociateFaces":  service.WrapOp(h.handleDisassociateFaces),
		"SearchUsers":        service.WrapOp(h.handleSearchUsers),
		"SearchUsersByImage": service.WrapOp(h.handleSearchUsersByImage),
	}
}

// =============================================================================
// Users
// =============================================================================

type createUserReq struct {
	CollectionId string `json:"CollectionId"` //nolint:revive,staticcheck // existing issue.
	UserId       string `json:"UserId"`       //nolint:revive,staticcheck // existing issue.
}

func (h *Handler) handleCreateUser(_ context.Context, req *createUserReq) (*struct{}, error) {
	if req.CollectionId == "" {
		return nil, fmt.Errorf("%w: CollectionId is required", ErrValidation)
	}

	if req.UserId == "" {
		return nil, fmt.Errorf("%w: UserId is required", ErrValidation)
	}

	if err := h.Backend.CreateUser(req.CollectionId, req.UserId); err != nil {
		return nil, err
	}

	return &struct{}{}, nil
}

type deleteUserReq struct {
	CollectionId string `json:"CollectionId"` //nolint:revive,staticcheck // existing issue.
	UserId       string `json:"UserId"`       //nolint:revive,staticcheck // existing issue.
}

func (h *Handler) handleDeleteUser(_ context.Context, req *deleteUserReq) (*struct{}, error) {
	if req.CollectionId == "" {
		return nil, fmt.Errorf("%w: CollectionId is required", ErrValidation)
	}

	if req.UserId == "" {
		return nil, fmt.Errorf("%w: UserId is required", ErrValidation)
	}

	if err := h.Backend.DeleteUser(req.CollectionId, req.UserId); err != nil {
		return nil, err
	}

	return &struct{}{}, nil
}

type listUsersReq struct {
	CollectionId string `json:"CollectionId"` //nolint:revive,staticcheck // existing issue.
	NextToken    string `json:"NextToken"`
	MaxResults   int32  `json:"MaxResults"`
}

type userEntry struct {
	UserId     string `json:"UserId"` //nolint:revive,staticcheck // existing issue.
	UserStatus string `json:"UserStatus"`
}

type listUsersResp struct {
	NextToken string      `json:"NextToken,omitempty"`
	Users     []userEntry `json:"Users"`
}

func (h *Handler) handleListUsers(_ context.Context, req *listUsersReq) (*listUsersResp, error) {
	if req.CollectionId == "" {
		return nil, fmt.Errorf("%w: CollectionId is required", ErrValidation)
	}

	users, nextToken, err := h.Backend.ListUsers(req.CollectionId, req.MaxResults, req.NextToken)
	if err != nil {
		return nil, err
	}

	entries := make([]userEntry, 0, len(users))
	for _, u := range users {
		entries = append(entries, userEntry{
			UserId:     u.UserID,
			UserStatus: u.UserStatus,
		})
	}

	return &listUsersResp{
		Users:     entries,
		NextToken: nextToken,
	}, nil
}

type associateFacesReq struct {
	CollectionId string   `json:"CollectionId"` //nolint:revive,staticcheck // existing issue.
	UserId       string   `json:"UserId"`       //nolint:revive,staticcheck // existing issue.
	FaceIds      []string `json:"FaceIds"`      //nolint:revive // existing issue.
}

type associatedFaceEntry struct {
	FaceId string `json:"FaceId"` //nolint:revive,staticcheck // existing issue.
}

type unsuccessfulFaceAssociationEntry struct {
	FaceId  string   `json:"FaceId"` //nolint:revive,staticcheck // existing issue.
	Reasons []string `json:"Reasons"`
}

type associateFacesResp struct {
	AssociatedFaces              []associatedFaceEntry              `json:"AssociatedFaces"`
	UnsuccessfulFaceAssociations []unsuccessfulFaceAssociationEntry `json:"UnsuccessfulFaceAssociations"`
}

func (h *Handler) handleAssociateFaces( //nolint:dupl // existing issue.
	_ context.Context,
	req *associateFacesReq,
) (*associateFacesResp, error) {
	if req.CollectionId == "" {
		return nil, fmt.Errorf("%w: CollectionId is required", ErrValidation)
	}

	if req.UserId == "" {
		return nil, fmt.Errorf("%w: UserId is required", ErrValidation)
	}

	associated, unsuccessful, err := h.Backend.AssociateFaces(req.CollectionId, req.UserId, req.FaceIds)
	if err != nil {
		return nil, err
	}

	assocEntries := make([]associatedFaceEntry, 0, len(associated))
	for _, a := range associated {
		assocEntries = append(assocEntries, associatedFaceEntry{FaceId: a.FaceID})
	}

	unsuccessfulEntries := make([]unsuccessfulFaceAssociationEntry, 0, len(unsuccessful))
	for _, u := range unsuccessful {
		unsuccessfulEntries = append(unsuccessfulEntries, unsuccessfulFaceAssociationEntry{
			FaceId:  u.FaceID,
			Reasons: u.Reasons,
		})
	}

	return &associateFacesResp{
		AssociatedFaces:              assocEntries,
		UnsuccessfulFaceAssociations: unsuccessfulEntries,
	}, nil
}

type disassociateFacesReq struct {
	CollectionId string   `json:"CollectionId"` //nolint:revive,staticcheck // existing issue.
	UserId       string   `json:"UserId"`       //nolint:revive,staticcheck // existing issue.
	FaceIds      []string `json:"FaceIds"`      //nolint:revive // existing issue.
}

type disassociatedFaceEntry struct {
	FaceId string `json:"FaceId"` //nolint:revive,staticcheck // existing issue.
}

type unsuccessfulFaceDisassociationEntry struct {
	FaceId  string   `json:"FaceId"` //nolint:revive,staticcheck // existing issue.
	Reasons []string `json:"Reasons"`
}

type disassociateFacesResp struct {
	DisassociatedFaces              []disassociatedFaceEntry              `json:"DisassociatedFaces"`
	UnsuccessfulFaceDisassociations []unsuccessfulFaceDisassociationEntry `json:"UnsuccessfulFaceDisassociations"`
}

func (h *Handler) handleDisassociateFaces( //nolint:dupl // existing issue.
	_ context.Context, req *disassociateFacesReq,
) (*disassociateFacesResp, error) {
	if req.CollectionId == "" {
		return nil, fmt.Errorf("%w: CollectionId is required", ErrValidation)
	}

	if req.UserId == "" {
		return nil, fmt.Errorf("%w: UserId is required", ErrValidation)
	}

	disassociated, unsuccessful, err := h.Backend.DisassociateFaces(
		req.CollectionId, req.UserId, req.FaceIds,
	)
	if err != nil {
		return nil, err
	}

	disassocEntries := make([]disassociatedFaceEntry, 0, len(disassociated))
	for _, d := range disassociated {
		disassocEntries = append(disassocEntries, disassociatedFaceEntry{FaceId: d.FaceID})
	}

	unsuccessfulEntries := make([]unsuccessfulFaceDisassociationEntry, 0, len(unsuccessful))
	for _, u := range unsuccessful {
		unsuccessfulEntries = append(unsuccessfulEntries, unsuccessfulFaceDisassociationEntry{
			FaceId:  u.FaceID,
			Reasons: u.Reasons,
		})
	}

	return &disassociateFacesResp{
		DisassociatedFaces:              disassocEntries,
		UnsuccessfulFaceDisassociations: unsuccessfulEntries,
	}, nil
}

type searchUsersReq struct {
	CollectionId string `json:"CollectionId"` //nolint:revive,staticcheck // existing issue.
	UserId       string `json:"UserId"`       //nolint:revive,staticcheck // existing issue.
	FaceId       string `json:"FaceId"`       //nolint:revive,staticcheck // existing issue.
	MaxUsers     int32  `json:"MaxUsers"`
}

type userMatchEntry struct {
	User       userEntry `json:"User"`
	Similarity float64   `json:"Similarity"`
}

type searchedFaceEntry struct {
	FaceId string `json:"FaceId"` //nolint:revive,staticcheck // existing issue.
}

type searchUsersResp struct {
	FaceModelVersion string             `json:"FaceModelVersion"`
	SearchedUser     *userEntry         `json:"SearchedUser,omitempty"`
	SearchedFace     *searchedFaceEntry `json:"SearchedFace,omitempty"`
	UserMatches      []userMatchEntry   `json:"UserMatches"`
}

// handleSearchUsers. Real SearchUsersInput marks only CollectionId required
// (rekognition@v1.54.4 api_op_SearchUsers.go): "The request must be
// provided with either FaceId or UserId" -- FaceId need not already be
// associated with a User. Previously only UserId was accepted --
// gopherstack-2wvq.
func (h *Handler) handleSearchUsers(_ context.Context, req *searchUsersReq) (*searchUsersResp, error) {
	if req.CollectionId == "" {
		return nil, fmt.Errorf("%w: CollectionId is required", ErrValidation)
	}

	if req.UserId == "" && req.FaceId == "" {
		return nil, fmt.Errorf("%w: UserId or FaceId is required", ErrValidation)
	}

	var (
		matches []*UserMatch
		err     error
	)

	if req.UserId != "" {
		matches, err = h.Backend.SearchUsers(req.CollectionId, req.UserId, req.MaxUsers)
	} else {
		matches, err = h.Backend.SearchUsersByFace(req.CollectionId, req.FaceId, req.MaxUsers)
	}
	if err != nil {
		return nil, err
	}

	entries := make([]userMatchEntry, 0, len(matches))
	for _, m := range matches {
		entries = append(entries, userMatchEntry{
			Similarity: m.Similarity,
			User: userEntry{
				UserId:     m.User.UserID,
				UserStatus: m.User.UserStatus,
			},
		})
	}

	resp := &searchUsersResp{
		FaceModelVersion: faceModelVersion,
		UserMatches:      entries,
	}

	if req.UserId != "" {
		resp.SearchedUser = &userEntry{UserId: req.UserId, UserStatus: "ACTIVE"}
	} else {
		resp.SearchedFace = &searchedFaceEntry{FaceId: req.FaceId}
	}

	return resp, nil
}

type searchUsersByImageReq struct {
	CollectionId string   `json:"CollectionId"` //nolint:revive,staticcheck // existing issue.
	Image        imageRef `json:"Image"`
	MaxUsers     int32    `json:"MaxUsers"`
}

type searchUsersByImageResp struct {
	FaceModelVersion string           `json:"FaceModelVersion"`
	UserMatches      []userMatchEntry `json:"UserMatches"`
}

func (h *Handler) handleSearchUsersByImage(
	ctx context.Context, req *searchUsersByImageReq,
) (*searchUsersByImageResp, error) {
	if req.CollectionId == "" {
		return nil, fmt.Errorf("%w: CollectionId is required", ErrValidation)
	}

	if err := h.checkImageRef(ctx, req.Image); err != nil {
		return nil, err
	}

	matches, err := h.Backend.SearchUsersByImage(req.CollectionId, req.MaxUsers, imageRefKey(req.Image))
	if err != nil {
		return nil, err
	}

	entries := make([]userMatchEntry, 0, len(matches))
	for _, m := range matches {
		entries = append(entries, userMatchEntry{
			Similarity: m.Similarity,
			User: userEntry{
				UserId:     m.User.UserID,
				UserStatus: m.User.UserStatus,
			},
		})
	}

	return &searchUsersByImageResp{
		FaceModelVersion: faceModelVersion,
		UserMatches:      entries,
	}, nil
}
