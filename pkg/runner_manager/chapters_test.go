package runner_manager

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"testing"

	"go.uber.org/mock/gomock"
	"gorm.io/gorm"

	"github.com/tum-dev/gocast/runner/pkg/ptr"
	"github.com/tum-dev/gocast/runner/protobuf"

	"github.com/TUM-Dev/gocast/dao"
	"github.com/TUM-Dev/gocast/mock_dao"
	"github.com/TUM-Dev/gocast/model"
	"github.com/TUM-Dev/gocast/tools"
)

func TestHandleVODReadyImportsChapters(t *testing.T) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(tools.SetJWTKeyForTest(key))

	notification := &protobuf.VODReadyNotification{
		Stream:        &protobuf.StreamInfo{Id: ptr.Take(uint64(5))},
		StreamVersion: ptr.Take(protobuf.StreamVersion_STREAM_VERSION_COMBINED),
		Url:           ptr.Take("http://edge/vod/5/x/playlist.m3u8"),
		Chapters: []*protobuf.Chapter{
			{StartSeconds: ptr.Take(uint32(0)), Title: ptr.Take("Intro")},
			{StartSeconds: ptr.Take(uint32(3725)), Title: ptr.Take(" ")},
		},
	}
	setup := func(ctrl *gomock.Controller, sections *mock_dao.MockVideoSectionDao) *Manager {
		streamsDao := mock_dao.NewMockStreamsDao(ctrl)
		streamsDao.EXPECT().GetStreamByID(gomock.Any(), "5").Return(model.Stream{Model: gorm.Model{ID: 5}, Recording: true}, nil)
		streamsDao.EXPECT().SaveStream(gomock.Any()).Return(nil)
		streamsDao.EXPECT().RemoveTranscodingProgress(gomock.Any(), uint(5)).Return(nil)
		// No runner, so the images are skipped and only logged.
		runnerDao := mock_dao.NewMockRunnerDao(ctrl)
		runnerDao.EXPECT().ReserveRunner(gomock.Any()).Return(model.Runner{}, nil).AnyTimes()
		return New(withoutCut(ctrl, dao.DaoWrapper{StreamsDao: streamsDao, RunnerDao: runnerDao, VideoSectionDao: sections}))
	}

	t.Run("a lecture without sections gets the chapters", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		sections := mock_dao.NewMockVideoSectionDao(ctrl)
		var created []model.VideoSection
		gomock.InOrder(
			sections.EXPECT().GetByStreamId(uint(5)).Return(nil, nil),
			sections.EXPECT().Create(gomock.Any()).DoAndReturn(func(s []model.VideoSection) error { created = s; return nil }),
			sections.EXPECT().GetByStreamId(uint(5)).DoAndReturn(func(uint) ([]model.VideoSection, error) { return created, nil }),
		)
		if _, err := setup(ctrl, sections).handleVODReady(context.Background(), notification); err != nil {
			t.Fatal(err)
		}
		want := []model.VideoSection{
			{StreamID: 5, Description: "Intro"},
			{StreamID: 5, Description: "Chapter 2", StartHours: 1, StartMinutes: 2, StartSeconds: 5},
		}
		if len(created) != len(want) || created[0] != want[0] || created[1] != want[1] {
			t.Errorf("created %+v, want %+v", created, want)
		}
	})

	t.Run("a lecture with sections keeps them", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		sections := mock_dao.NewMockVideoSectionDao(ctrl)
		sections.EXPECT().GetByStreamId(uint(5)).Return([]model.VideoSection{{StreamID: 5, Description: "edited"}}, nil)
		// no Create expected
		if _, err := setup(ctrl, sections).handleVODReady(context.Background(), notification); err != nil {
			t.Fatal(err)
		}
	})
}
