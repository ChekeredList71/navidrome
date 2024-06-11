package scanner2

import (
	"context"
	"time"

	"github.com/navidrome/navidrome/model"
	"github.com/navidrome/navidrome/tests"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("phaseMissingTracks", func() {
	var (
		phase *phaseMissingTracks
		ctx   context.Context
		ds    model.DataStore
		mr    *tests.MockMediaFileRepo
		lr    *tests.MockLibraryRepo
	)

	BeforeEach(func() {
		ctx = context.Background()
		mr = tests.CreateMockMediaFileRepo()
		lr = &tests.MockLibraryRepo{}
		lr.SetData(model.Libraries{{ID: 1, LastScanStartedAt: time.Date(2021, 1, 1, 0, 0, 0, 0, time.UTC)}})
		ds = &tests.MockDataStore{MockedMediaFile: mr, MockedLibrary: lr}
		phase = createPhaseMissingTracks(ctx, ds)
	})

	Describe("produceMissingTracks", func() {
		var (
			put      func(tracks *missingTracks)
			produced []*missingTracks
		)

		BeforeEach(func() {
			produced = nil
			put = func(tracks *missingTracks) {
				produced = append(produced, tracks)
			}
		})

		When("there are no missing tracks", func() {
			It("should not call put", func() {
				mr.SetData(model.MediaFiles{
					{ID: "1", PID: "A", Missing: false},
					{ID: "2", PID: "A", Missing: false},
				})

				err := phase.produce(put)
				Expect(err).ToNot(HaveOccurred())
				Expect(produced).To(BeEmpty())
			})
		})

		When("there are missing tracks", func() {
			It("should call put for any missing tracks with corresponding matches", func() {
				mr.SetData(model.MediaFiles{
					{ID: "1", PID: "A", Missing: true, LibraryID: 1},
					{ID: "2", PID: "B", Missing: true, LibraryID: 1},
					{ID: "3", PID: "A", Missing: false, LibraryID: 1},
				})

				err := phase.produce(put)
				Expect(err).ToNot(HaveOccurred())
				Expect(produced).To(HaveLen(1))
				Expect(produced[0].pid).To(Equal("A"))
				Expect(produced[0].missing).To(HaveLen(1))
				Expect(produced[0].matched).To(HaveLen(1))
			})
			It("should not call put if there are no matches for any missing tracks", func() {
				mr.SetData(model.MediaFiles{
					{ID: "1", PID: "A", Missing: true, LibraryID: 1},
					{ID: "2", PID: "B", Missing: true, LibraryID: 1},
					{ID: "3", PID: "C", Missing: false, LibraryID: 1},
				})

				err := phase.produce(put)
				Expect(err).ToNot(HaveOccurred())
				Expect(produced).To(BeZero())
			})
		})
	})

	Describe("processMissingTracks", func() {
		It("should move the matched track when the missing track is the exact same", func() {
			missingTrack := model.MediaFile{ID: "1", PID: "A", Path: "dir1/path1.mp3", Tags: model.Tags{"title": []string{"title1"}}, Size: 100}
			matchedTrack := model.MediaFile{ID: "2", PID: "A", Path: "dir2/path2.mp3", Tags: model.Tags{"title": []string{"title1"}}, Size: 100}

			_ = ds.MediaFile(ctx).Put(&missingTrack)
			_ = ds.MediaFile(ctx).Put(&matchedTrack)

			in := &missingTracks{
				missing: []model.MediaFile{missingTrack},
				matched: []model.MediaFile{matchedTrack},
			}

			_, err := phase.processMissingTracks(in)
			Expect(err).ToNot(HaveOccurred())

			movedTrack, _ := ds.MediaFile(ctx).Get("1")
			Expect(movedTrack.Path).To(Equal(matchedTrack.Path))
		})

		It("should move the matched track when the missing track has the same tags and filename", func() {
			missingTrack := model.MediaFile{ID: "1", PID: "A", Path: "path1.mp3", Tags: model.Tags{"title": []string{"title1"}}, Size: 100}
			matchedTrack := model.MediaFile{ID: "2", PID: "A", Path: "path1.flac", Tags: model.Tags{"title": []string{"title1"}}, Size: 200}

			_ = ds.MediaFile(ctx).Put(&missingTrack)
			_ = ds.MediaFile(ctx).Put(&matchedTrack)

			in := &missingTracks{
				missing: []model.MediaFile{missingTrack},
				matched: []model.MediaFile{matchedTrack},
			}

			_, err := phase.processMissingTracks(in)
			Expect(err).ToNot(HaveOccurred())

			movedTrack, _ := ds.MediaFile(ctx).Get("1")
			Expect(movedTrack.Path).To(Equal(matchedTrack.Path))
			Expect(movedTrack.Size).To(Equal(matchedTrack.Size))
		})

		It("should prioritize exact matches", func() {
			missingTrack := model.MediaFile{ID: "1", PID: "A", Path: "dir1/file1.mp3", Tags: model.Tags{"title": []string{"title1"}}, Size: 100}
			matchedEquivalent := model.MediaFile{ID: "2", PID: "A", Path: "dir1/file1.flac", Tags: model.Tags{"title": []string{"title1"}}, Size: 200}
			matchedExact := model.MediaFile{ID: "3", PID: "A", Path: "dir2/file2.mp3", Tags: model.Tags{"title": []string{"title1"}}, Size: 100}

			_ = ds.MediaFile(ctx).Put(&missingTrack)
			_ = ds.MediaFile(ctx).Put(&matchedEquivalent)
			_ = ds.MediaFile(ctx).Put(&matchedExact)

			in := &missingTracks{
				missing: []model.MediaFile{missingTrack},
				// Note that equivalent comes before the exact match
				matched: []model.MediaFile{matchedEquivalent, matchedExact},
			}

			_, err := phase.processMissingTracks(in)
			Expect(err).ToNot(HaveOccurred())

			movedTrack, _ := ds.MediaFile(ctx).Get("1")
			Expect(movedTrack.Path).To(Equal(matchedExact.Path))
			Expect(movedTrack.Size).To(Equal(matchedExact.Size))
		})

		It("should return an error when there's an error moving the matched track", func() {
			missingTrack := model.MediaFile{ID: "1", PID: "A", Path: "path1.mp3", Tags: model.Tags{"title": []string{"title1"}}}
			matchedTrack := model.MediaFile{ID: "2", PID: "A", Path: "path1.mp3", Tags: model.Tags{"title": []string{"title1"}}}

			_ = ds.MediaFile(ctx).Put(&missingTrack)
			_ = ds.MediaFile(ctx).Put(&matchedTrack)

			in := &missingTracks{
				missing: []model.MediaFile{missingTrack},
				matched: []model.MediaFile{matchedTrack},
			}

			// Simulate an error when moving the matched track by deleting the track from the DB
			_ = ds.MediaFile(ctx).Delete("2")

			_, err := phase.processMissingTracks(in)
			Expect(err).To(HaveOccurred())
		})
	})
})
