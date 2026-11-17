package runner_manager

import (
	"context"
	"fmt"
	"strings"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/tum-dev/gocast/runner/protobuf"

	"github.com/TUM-Dev/gocast/model"
	"github.com/TUM-Dev/gocast/tools"
)

// importChapters makes the chapter marks of an uploaded file the lecture's sections. A
// lecture that has sections keeps them: another version or a re-upload must neither add
// the chapters twice nor replace sections someone has edited since. That also makes a
// retried VODReady harmless.
func (m *Manager) importChapters(ctx context.Context, stream model.Stream, n *protobuf.VODReadyNotification) error {
	if len(n.GetChapters()) == 0 {
		return nil
	}
	existing, err := m.dao.VideoSectionDao.GetByStreamId(stream.ID)
	if err != nil {
		return status.Errorf(codes.Internal, "can't look up the sections: %v", err)
	}
	if len(existing) > 0 {
		return nil
	}

	sections := make([]model.VideoSection, 0, len(n.GetChapters()))
	for i, c := range n.GetChapters() {
		title := strings.TrimSpace(c.GetTitle())
		if title == "" {
			title = fmt.Sprintf("Chapter %d", i+1)
		}
		s := c.GetStartSeconds()
		sections = append(sections, model.VideoSection{
			StreamID:     stream.ID,
			Description:  title,
			StartHours:   uint(s / 3600),
			StartMinutes: uint(s % 3600 / 60),
			StartSeconds: uint(s % 60),
		})
	}
	if err := m.dao.VideoSectionDao.Create(sections); err != nil {
		return status.Errorf(codes.Internal, "can't save the chapters: %v", err)
	}
	m.logger.Info("imported chapters", "stream", stream.ID, "chapters", len(sections))

	// The thumbnails are best effort, as for sections added by hand. Create fills the IDs
	// into a copy of the slice, so they are read back.
	created, err := m.dao.VideoSectionDao.GetByStreamId(stream.ID)
	if err != nil {
		m.logger.Error("can't read back imported chapters", "stream", stream.ID, "err", err)
		return nil
	}
	if err := tools.SetSignedPlaylists(&stream, &model.User{}, false); err != nil {
		m.logger.Error("can't sign playlist for chapter images", "stream", stream.ID, "err", err)
		return nil
	}
	// The version that brought the chapters, as the lecture may have no other yet.
	playlist := map[protobuf.StreamVersion]string{
		protobuf.StreamVersion_STREAM_VERSION_COMBINED:     stream.PlaylistUrl,
		protobuf.StreamVersion_STREAM_VERSION_PRESENTATION: stream.PlaylistUrlPRES,
		protobuf.StreamVersion_STREAM_VERSION_CAMERA:       stream.PlaylistUrlCAM,
	}[n.GetStreamVersion()]
	if err := m.RequestSectionImages(ctx, SectionImageRequest{StreamID: stream.ID, PlaylistURL: playlist, Sections: created}); err != nil {
		m.logger.Error("can't request chapter images", "stream", stream.ID, "err", err)
	}
	return nil
}
