package ecr

import (
	"context"
	"encoding/base64"
)

// repositoryView is the JSON representation of a repository.
// createdAt is serialised as a Unix epoch float64 (seconds) so that the AWS
// SDK v2 deserialiser, which expects a JSON Number for timestamp fields, can
// decode it correctly.
type repositoryView struct {
	EncryptionConfiguration            *encryptionConfigurationView   `json:"encryptionConfiguration,omitempty"`
	ImageTagMutability                 string                         `json:"imageTagMutability,omitempty"`
	RegistryID                         string                         `json:"registryId"`
	RepositoryARN                      string                         `json:"repositoryArn"`
	RepositoryName                     string                         `json:"repositoryName"`
	RepositoryURI                      string                         `json:"repositoryUri"`
	ImageTagMutabilityExclusionFilters []imageTagMutabilityFilterView `json:"imageTagMutabilityExclusionFilters,omitempty"`
	CreatedAt                          float64                        `json:"createdAt"`
	ImageScanningConfiguration         imageScanningConfigurationView `json:"imageScanningConfiguration"`
}

type imageScanningConfigurationView struct {
	ScanOnPush bool `json:"scanOnPush"`
}

type encryptionConfigurationView struct {
	EncryptionType string `json:"encryptionType"`
	KMSKey         string `json:"kmsKey,omitempty"`
}

type imageTagMutabilityFilterView struct {
	Filter     string `json:"filter,omitempty"`
	FilterType string `json:"filterType,omitempty"`
}

func toRepositoryView(r Repository) repositoryView {
	filters := make([]imageTagMutabilityFilterView, 0, len(r.ImageTagMutabilityExclusionFilters))
	for _, filter := range r.ImageTagMutabilityExclusionFilters {
		filters = append(filters, imageTagMutabilityFilterView(filter))
	}

	return repositoryView{
		CreatedAt: float64(r.CreatedAt.Unix()),
		EncryptionConfiguration: &encryptionConfigurationView{
			EncryptionType: r.EncryptionType,
			KMSKey:         r.KMSKey,
		},
		ImageScanningConfiguration: imageScanningConfigurationView{
			ScanOnPush: r.ScanOnPush,
		},
		ImageTagMutability:                 r.ImageTagMutability,
		ImageTagMutabilityExclusionFilters: filters,
		RegistryID:                         r.RegistryID,
		RepositoryARN:                      r.RepositoryARN,
		RepositoryName:                     r.RepositoryName,
		RepositoryURI:                      r.RepositoryURI,
	}
}

// createRepositoryInput is the request body for CreateRepository.
type createRepositoryInput struct {
	EncryptionConfiguration    *encryptionConfigurationView    `json:"encryptionConfiguration,omitempty"`
	ImageScanningConfiguration *imageScanningConfigurationView `json:"imageScanningConfiguration,omitempty"`
	RepositoryName             string                          `json:"repositoryName"`
	ImageTagMutability         string                          `json:"imageTagMutability,omitempty"`
	Tags                       []tagView                       `json:"tags,omitempty"`
}

type createRepositoryOutput struct {
	Repository repositoryView `json:"repository"`
}

func (h *Handler) handleCreateRepository(
	ctx context.Context,
	in *createRepositoryInput,
) (*createRepositoryOutput, error) {
	scanOnPush := false
	if in.ImageScanningConfiguration != nil {
		scanOnPush = in.ImageScanningConfiguration.ScanOnPush
	}

	encryptionType := ""
	kmsKey := ""
	if in.EncryptionConfiguration != nil {
		encryptionType = in.EncryptionConfiguration.EncryptionType
		kmsKey = in.EncryptionConfiguration.KMSKey
	}

	repo, err := h.Backend.CreateRepository(
		ctx, in.RepositoryName, in.ImageTagMutability, scanOnPush, encryptionType, kmsKey,
	)
	if err != nil {
		return nil, err
	}

	if len(in.Tags) > 0 {
		tagMap := make(map[string]string, len(in.Tags))
		for _, t := range in.Tags {
			tagMap[t.Key] = t.Value
		}

		if tagErr := h.Backend.TagResource(ctx, repo.RepositoryARN, tagMap); tagErr != nil {
			return nil, tagErr
		}
	}

	return &createRepositoryOutput{Repository: toRepositoryView(*repo)}, nil
}

// describeRepositoriesInput is the request body for DescribeRepositories.
type describeRepositoriesInput struct {
	NextToken       string   `json:"nextToken,omitempty"`
	RepositoryNames []string `json:"repositoryNames"`
	MaxResults      int      `json:"maxResults,omitempty"`
}

type describeRepositoriesOutput struct {
	NextToken    string           `json:"nextToken,omitempty"`
	Repositories []repositoryView `json:"repositories"`
}

func (h *Handler) handleDescribeRepositories(
	ctx context.Context,
	in *describeRepositoriesInput,
) (*describeRepositoriesOutput, error) {
	repos, err := h.Backend.DescribeRepositories(ctx, in.RepositoryNames)
	if err != nil {
		return nil, err
	}

	// Apply nextToken cursor: token is base64(repoName) of the first repo on this page.
	if in.NextToken != "" && len(in.RepositoryNames) == 0 {
		decoded, decErr := base64.StdEncoding.DecodeString(in.NextToken)
		if decErr == nil {
			cursorName := string(decoded)
			start := 0
			for i, r := range repos {
				if r.RepositoryName == cursorName {
					start = i

					break
				}
			}

			repos = repos[start:]
		}
	}

	// Apply maxResults page limit; emit opaque token = base64(next repo name).
	maxResults := in.MaxResults
	if maxResults <= 0 {
		maxResults = 100 // AWS default when maxResults is not used.
	}

	var nextToken string
	if len(repos) > maxResults {
		nextToken = base64.StdEncoding.EncodeToString([]byte(repos[maxResults].RepositoryName))
		repos = repos[:maxResults]
	}

	views := make([]repositoryView, 0, len(repos))
	for _, r := range repos {
		views = append(views, toRepositoryView(r))
	}

	return &describeRepositoriesOutput{Repositories: views, NextToken: nextToken}, nil
}

// deleteRepositoryInput is the request body for DeleteRepository.
type deleteRepositoryInput struct {
	RepositoryName string `json:"repositoryName"`
	RegistryID     string `json:"registryId,omitempty"`
	Force          bool   `json:"force,omitempty"`
}

type deleteRepositoryOutput struct {
	Repository repositoryView `json:"repository"`
}

func (h *Handler) handleDeleteRepository(
	ctx context.Context,
	in *deleteRepositoryInput,
) (*deleteRepositoryOutput, error) {
	repo, err := h.Backend.DeleteRepository(ctx, in.RepositoryName, in.Force)
	if err != nil {
		return nil, err
	}

	return &deleteRepositoryOutput{Repository: toRepositoryView(*repo)}, nil
}
