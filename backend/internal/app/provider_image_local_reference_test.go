package app

import (
	"context"
	"encoding/base64"
	"testing"

	"yingce/backend/internal/model"
	"yingce/backend/internal/protocol"
)

func TestOpenAIImageLocalReferencesWithoutPublicURL(t *testing.T) {
	service, _, imageBytes := cloudAgentVisionFixture(t)
	t.Setenv("CANVAS_PUBLIC_BASE_URL", "")
	input := canvasGenerationInput{
		Mode: "image", Prompt: "Keep the character consistent with @图片1",
		Config:          providerConfig{InterfaceType: string(model.ChannelInterfaceOpenAIImage), Model: "test-image"},
		ReferenceImages: []providerMedia{{ID: "character", StorageKey: "resource:ref-one"}},
	}
	ctx := ensureOfficialProtocolAdapter(context.Background(), input.Config.InterfaceType)
	policy := providerMediaHydrationPolicyFor(ctx, input)
	if policy.requireURL {
		t.Fatal("local OpenAI image references must not require a public URL")
	}
	if err := service.hydrateGenerationMedia("user", &input, policy); err != nil {
		t.Fatal(err)
	}
	want := "data:image/png;base64," + base64.StdEncoding.EncodeToString(imageBytes)
	if input.ReferenceImages[0].DataURL != want || input.ReferenceImages[0].URL != "" {
		t.Fatal("reference did not hydrate from the authorized local resource")
	}
	adapter, ok := declarativeProtocolAdapterForContext(ctx, input.Config.InterfaceType)
	if !ok {
		t.Fatal("OpenAI image adapter is missing")
	}
	request, err := adapter.BuildCreate(ctx, protocol.RequestContext{Request: protocolRequestFromInput(input)})
	if err != nil {
		t.Fatal(err)
	}
	if request.Path != "/v1/images/edits" || request.ContentType != "multipart/form-data" || len(request.Files) != 1 || request.Files[0].Name != "image" || request.Files[0].Reference.DataURL != want {
		t.Fatal("image edit request did not retain inline image bytes as a multipart file")
	}
	foreign := canvasGenerationInput{ReferenceImages: []providerMedia{{StorageKey: "resource:ref-one"}}}
	if err := service.hydrateGenerationMedia("other-user", &foreign, policy); err == nil {
		t.Fatal("another user's reference resource was accepted")
	}
}

func TestNewAPIChannel2LocalVideoReferencesUseInlineDataURL(t *testing.T) {
	service, _, imageBytes := cloudAgentVisionFixture(t)
	t.Setenv("CANVAS_PUBLIC_BASE_URL", "")
	registry, err := protocol.NewRegistry(protocol.UnavailableAdapter{Info: protocol.Metadata{
		ID: string(model.ChannelInterfaceNewAPIChannel2), Version: "test", RequiresPublicMediaURLs: true,
	}})
	if err != nil {
		t.Fatal(err)
	}
	ctx := withProtocolRegistry(context.Background(), registry)
	input := canvasGenerationInput{
		Mode: "video",
		Config: providerConfig{
			Model: "sd2.0-fast", InterfaceType: string(model.ChannelInterfaceNewAPIChannel2), VideoSeconds: "6",
		},
		ReferenceImages: []providerMedia{{ID: "first-frame", StorageKey: "resource:ref-one"}},
	}
	policy := providerMediaHydrationPolicyFor(ctx, input)
	if policy.requireURL || !policy.preferURL {
		t.Fatalf("NewAPI Channel 2 media policy = %#v", policy)
	}
	if err := service.hydrateGenerationMedia("user", &input, policy); err != nil {
		t.Fatal(err)
	}
	want := "data:image/png;base64," + base64.StdEncoding.EncodeToString(imageBytes)
	if input.ReferenceImages[0].DataURL != want || input.ReferenceImages[0].URL != "" {
		t.Fatalf("reference media = %#v", input.ReferenceImages[0])
	}
	body := officialVideoCreateBody(t, input)
	images, _ := body["image_urls"].([]any)
	if len(images) != 1 || images[0] != want {
		t.Fatalf("image_urls = %#v, want one inline local reference", images)
	}
}
