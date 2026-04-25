package provider

import (
	"fmt"
	"strings"

	"github.com/JiaCheng2004/Polaris/internal/modality"
)

func (r *Registry) Count() int {
	return len(r.models)
}

func (r *Registry) ProviderCount() int {
	providers := map[string]struct{}{}
	for _, model := range r.models {
		if model.Provider == "" {
			continue
		}
		providers[model.Provider] = struct{}{}
	}
	return len(providers)
}

func (r *Registry) GetChatAdapter(name string) (modality.ChatAdapter, Model, error) {
	model, err := r.RequireModel(name, modality.ModalityChat)
	if err != nil {
		return nil, Model{}, err
	}
	adapter, ok := r.chatAdapters[model.ID]
	if !ok {
		return nil, Model{}, fmt.Errorf("%w: %s", ErrAdapterMissing, model.ID)
	}
	return adapter, model, nil
}

func (r *Registry) GetEmbedAdapter(name string) (modality.EmbedAdapter, Model, error) {
	model, err := r.RequireModel(name, modality.ModalityEmbed)
	if err != nil {
		return nil, Model{}, err
	}
	adapter, ok := r.embedAdapters[model.ID]
	if !ok {
		return nil, Model{}, fmt.Errorf("%w: %s", ErrAdapterMissing, model.ID)
	}
	return adapter, model, nil
}

func (r *Registry) GetImageAdapter(name string) (modality.ImageAdapter, Model, error) {
	model, err := r.RequireModel(name, modality.ModalityImage)
	if err != nil {
		return nil, Model{}, err
	}
	adapter, ok := r.imageAdapters[model.ID]
	if !ok {
		return nil, Model{}, fmt.Errorf("%w: %s", ErrAdapterMissing, model.ID)
	}
	return adapter, model, nil
}

func (r *Registry) GetVoiceAdapter(name string) (modality.VoiceAdapter, Model, error) {
	model, err := r.RequireModel(name, modality.ModalityVoice)
	if err != nil {
		return nil, Model{}, err
	}
	adapter, ok := r.voiceAdapters[model.ID]
	if !ok {
		return nil, Model{}, fmt.Errorf("%w: %s", ErrAdapterMissing, model.ID)
	}
	return adapter, model, nil
}

func (r *Registry) GetTranslationAdapter(name string) (modality.TranslationAdapter, Model, error) {
	model, err := r.RequireModel(name, modality.ModalityTranslation)
	if err != nil {
		return nil, Model{}, err
	}
	adapter, ok := r.translationAdapters[model.ID]
	if !ok {
		return nil, Model{}, fmt.Errorf("%w: %s", ErrAdapterMissing, model.ID)
	}
	return adapter, model, nil
}

func (r *Registry) GetVoiceCatalogAdapter(providerName string) (modality.VoiceCatalogAdapter, error) {
	trimmed := strings.TrimSpace(providerName)
	if trimmed == "" {
		return nil, fmt.Errorf("%w: provider", ErrAdapterMissing)
	}
	adapter, ok := r.voiceCatalogAdapters[trimmed]
	if !ok {
		return nil, fmt.Errorf("%w: %s", ErrAdapterMissing, trimmed)
	}
	return adapter, nil
}

func (r *Registry) GetVoiceAssetAdapter(providerName string) (modality.VoiceAssetAdapter, error) {
	trimmed := strings.TrimSpace(providerName)
	if trimmed == "" {
		return nil, fmt.Errorf("%w: provider", ErrAdapterMissing)
	}
	adapter, ok := r.voiceAssetAdapters[trimmed]
	if !ok {
		return nil, fmt.Errorf("%w: %s", ErrAdapterMissing, trimmed)
	}
	return adapter, nil
}

func (r *Registry) GetStreamingTranscriptionAdapter(name string) (modality.StreamingTranscriptionAdapter, Model, error) {
	model, err := r.RequireModel(name, modality.ModalityVoice, modality.CapabilityStreaming)
	if err != nil {
		return nil, Model{}, err
	}
	adapter, ok := r.streamingTranscriptionAdapters[model.ID]
	if !ok {
		return nil, Model{}, fmt.Errorf("%w: %s", ErrAdapterMissing, model.ID)
	}
	return adapter, model, nil
}

func (r *Registry) GetVideoAdapter(name string) (modality.VideoAdapter, Model, error) {
	model, err := r.RequireModel(name, modality.ModalityVideo)
	if err != nil {
		return nil, Model{}, err
	}
	adapter, ok := r.videoAdapters[model.ID]
	if !ok {
		return nil, Model{}, fmt.Errorf("%w: %s", ErrAdapterMissing, model.ID)
	}
	return adapter, model, nil
}

func (r *Registry) GetAudioAdapter(name string) (modality.AudioAdapter, Model, error) {
	model, err := r.RequireModel(name, modality.ModalityAudio)
	if err != nil {
		return nil, Model{}, err
	}
	adapter, ok := r.audioAdapters[model.ID]
	if !ok {
		return nil, Model{}, fmt.Errorf("%w: %s", ErrAdapterMissing, model.ID)
	}
	return adapter, model, nil
}

func (r *Registry) GetInterpretingAdapter(name string) (modality.InterpretingAdapter, Model, error) {
	model, err := r.RequireModel(name, modality.ModalityInterpreting)
	if err != nil {
		return nil, Model{}, err
	}
	adapter, ok := r.interpretingAdapters[model.ID]
	if !ok {
		return nil, Model{}, fmt.Errorf("%w: %s", ErrAdapterMissing, model.ID)
	}
	return adapter, model, nil
}

func (r *Registry) GetMusicAdapter(name string) (modality.MusicAdapter, Model, error) {
	model, err := r.RequireModel(name, modality.ModalityMusic)
	if err != nil {
		return nil, Model{}, err
	}
	adapter, ok := r.musicAdapters[model.ID]
	if !ok {
		return nil, Model{}, fmt.Errorf("%w: %s", ErrAdapterMissing, model.ID)
	}
	return adapter, model, nil
}

func (r *Registry) GetAudioNotesAdapter(name string) (modality.AudioNotesAdapter, Model, error) {
	model, err := r.RequireModel(name, modality.ModalityNotes)
	if err != nil {
		return nil, Model{}, err
	}
	adapter, ok := r.audioNotesAdapters[model.ID]
	if !ok {
		return nil, Model{}, fmt.Errorf("%w: %s", ErrAdapterMissing, model.ID)
	}
	return adapter, model, nil
}

func (r *Registry) GetPodcastAdapter(name string) (modality.PodcastAdapter, Model, error) {
	model, err := r.RequireModel(name, modality.ModalityPodcast)
	if err != nil {
		return nil, Model{}, err
	}
	adapter, ok := r.podcastAdapters[model.ID]
	if !ok {
		return nil, Model{}, fmt.Errorf("%w: %s", ErrAdapterMissing, model.ID)
	}
	return adapter, model, nil
}
