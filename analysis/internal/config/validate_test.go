// Copyright 2022 The LUCI Authors.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package config

import (
	"context"
	"io/ioutil"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
	"go.chromium.org/luci/config/validation"
	"google.golang.org/protobuf/encoding/prototext"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/durationpb"

	configpb "go.chromium.org/luci/analysis/proto/config"
)

func TestServiceConfigValidator(t *testing.T) {
	t.Parallel()

	validate := func(cfg *configpb.Config) error {
		c := validation.Context{Context: context.Background()}
		validateConfig(&c, cfg)
		return c.Finalize()
	}

	Convey("config template is valid", t, func() {
		content, err := ioutil.ReadFile(
			"../../configs/services/chops-weetbix-dev/config-template.cfg",
		)
		So(err, ShouldBeNil)
		cfg := &configpb.Config{}
		So(prototext.Unmarshal(content, cfg), ShouldBeNil)
		So(validate(cfg), ShouldBeNil)
	})

	Convey("valid config is valid", t, func() {
		cfg, err := CreatePlaceholderConfig()
		So(err, ShouldBeNil)

		So(validate(cfg), ShouldBeNil)
	})

	Convey("monorail hostname", t, func() {
		cfg, err := CreatePlaceholderConfig()
		So(err, ShouldBeNil)

		Convey("must be specified", func() {
			cfg.MonorailHostname = ""
			So(validate(cfg), ShouldErrLike, "empty value is not allowed")
		})
		Convey("must be correctly formed", func() {
			cfg.MonorailHostname = "monorail host"
			So(validate(cfg), ShouldErrLike, `invalid hostname: "monorail host"`)
		})
	})
	Convey("chunk GCS bucket", t, func() {
		cfg, err := CreatePlaceholderConfig()
		So(err, ShouldBeNil)

		Convey("must be specified", func() {
			cfg.ChunkGcsBucket = ""
			So(validate(cfg), ShouldErrLike, "empty chunk_gcs_bucket is not allowed")
		})
		Convey("must be correctly formed", func() {
			cfg, err := CreatePlaceholderConfig()
			So(err, ShouldBeNil)

			cfg.ChunkGcsBucket = "my bucket"
			So(validate(cfg), ShouldErrLike, `invalid chunk_gcs_bucket: "my bucket"`)
		})
	})
	Convey("reclustering workers", t, func() {
		cfg, err := CreatePlaceholderConfig()
		So(err, ShouldBeNil)

		Convey("less than zero", func() {
			cfg.ReclusteringWorkers = -1
			So(validate(cfg), ShouldErrLike, `value is less than zero`)
		})
		Convey("too large", func() {
			cfg.ReclusteringWorkers = 1001
			So(validate(cfg), ShouldErrLike, `value is greater than 1000`)
		})
	})
	Convey("reclustering interval", t, func() {
		cfg, err := CreatePlaceholderConfig()
		So(err, ShouldBeNil)

		Convey("less than zero", func() {
			cfg.ReclusteringIntervalMinutes = -1
			So(validate(cfg), ShouldErrLike, `value is less than zero`)
		})
		Convey("too large", func() {
			cfg.ReclusteringIntervalMinutes = 10
			So(validate(cfg), ShouldErrLike, `value is greater than 9`)
		})
	})
}

func TestProjectConfigValidator(t *testing.T) {
	t.Parallel()

	validate := func(cfg *configpb.ProjectConfig) error {
		c := validation.Context{Context: context.Background()}
		ValidateProjectConfig(&c, cfg)
		return c.Finalize()
	}

	Convey("config template is valid", t, func() {
		content, err := ioutil.ReadFile(
			"../../configs/projects/chromium/chops-weetbix-dev-template.cfg",
		)
		So(err, ShouldBeNil)
		cfg := &configpb.ProjectConfig{}
		So(prototext.Unmarshal(content, cfg), ShouldBeNil)
		So(validate(cfg), ShouldBeNil)
	})

	Convey("valid config is valid", t, func() {
		cfg := CreatePlaceholderProjectConfig()
		So(validate(cfg), ShouldBeNil)
	})

	Convey("monorail", t, func() {
		cfg := CreatePlaceholderProjectConfig()
		Convey("must be specified", func() {
			cfg.Monorail = nil
			So(validate(cfg), ShouldErrLike, "monorail must be specified")
		})

		Convey("project must be specified", func() {
			cfg.Monorail.Project = ""
			So(validate(cfg), ShouldErrLike, "empty project is not allowed")
		})

		Convey("illegal monorail project", func() {
			// Project does not satisfy regex.
			cfg.Monorail.Project = "-my-project"
			So(validate(cfg), ShouldErrLike, `invalid project: "-my-project"`)
		})

		Convey("negative priority field ID", func() {
			cfg.Monorail.PriorityFieldId = -1
			So(validate(cfg), ShouldErrLike, "value must be non-negative")
		})

		Convey("field value with negative field ID", func() {
			cfg.Monorail.DefaultFieldValues = []*configpb.MonorailFieldValue{
				{
					FieldId: -1,
					Value:   "",
				},
			}
			So(validate(cfg), ShouldErrLike, "value must be non-negative")
		})

		Convey("priorities", func() {
			priorities := cfg.Monorail.Priorities
			Convey("at least one must be specified", func() {
				cfg.Monorail.Priorities = nil
				So(validate(cfg), ShouldErrLike, "at least one monorail priority must be specified")
			})

			Convey("priority value is empty", func() {
				priorities[0].Priority = ""
				So(validate(cfg), ShouldErrLike, "empty value is not allowed")
			})

			Convey("threshold is not specified", func() {
				priorities[0].Threshold = nil
				So(validate(cfg), ShouldErrLike, "impact thresolds must be specified")
			})

			Convey("last priority thresholds must be satisfied by the bug-filing threshold", func() {
				lastPriority := priorities[len(priorities)-1]
				bugFilingThres := cfg.BugFilingThreshold

				// Test validation applies to all metrics.
				Convey("critical test failures exonerated", func() {
					bugFilingThres.CriticalFailuresExonerated = &configpb.MetricThreshold{OneDay: proto.Int64(70)}
					lastPriority.Threshold.CriticalFailuresExonerated = nil
					So(validate(cfg), ShouldErrLike, "critical_failures_exonerated / one_day): one_day threshold must be set, with a value of at most 70")
				})

				Convey("test results failed", func() {
					bugFilingThres.TestResultsFailed = &configpb.MetricThreshold{OneDay: proto.Int64(100)}
					lastPriority.Threshold.TestResultsFailed = nil
					So(validate(cfg), ShouldErrLike, "test_results_failed / one_day): one_day threshold must be set, with a value of at most 100")
				})

				Convey("test runs failed", func() {
					bugFilingThres.TestRunsFailed = &configpb.MetricThreshold{OneDay: proto.Int64(50)}
					lastPriority.Threshold.TestRunsFailed = nil
					So(validate(cfg), ShouldErrLike, "test_runs_failed / one_day): one_day threshold must be set, with a value of at most 50")
				})

				Convey("presubmit runs failed", func() {
					bugFilingThres.PresubmitRunsFailed = &configpb.MetricThreshold{OneDay: proto.Int64(10)}
					lastPriority.Threshold.PresubmitRunsFailed = nil
					So(validate(cfg), ShouldErrLike, "presubmit_runs_failed / one_day): one_day threshold must be set, with a value of at most 10")
				})

				// The following properties should hold for all metrics. We test
				// on one metric as the code is re-used for all metrics.
				Convey("one day threshold", func() {
					bugFilingThres.TestResultsFailed = &configpb.MetricThreshold{OneDay: proto.Int64(100)}
					lastPriority.Threshold.TestResultsFailed = &configpb.MetricThreshold{OneDay: proto.Int64(101)}
					So(validate(cfg), ShouldErrLike, "test_results_failed / one_day): value must be at most 100")
				})

				Convey("three day threshold", func() {
					bugFilingThres.TestResultsFailed = &configpb.MetricThreshold{ThreeDay: proto.Int64(300)}
					lastPriority.Threshold.TestResultsFailed = &configpb.MetricThreshold{ThreeDay: proto.Int64(301)}
					So(validate(cfg), ShouldErrLike, "test_results_failed / three_day): value must be at most 300")
				})

				Convey("seven day threshold", func() {
					bugFilingThres.TestResultsFailed = &configpb.MetricThreshold{SevenDay: proto.Int64(700)}
					lastPriority.Threshold.TestResultsFailed = &configpb.MetricThreshold{SevenDay: proto.Int64(701)}
					So(validate(cfg), ShouldErrLike, "test_results_failed / seven_day): value must be at most 700")
				})

				Convey("one day-filing threshold implies seven-day keep open threshold", func() {
					// Verify implications work across time.
					bugFilingThres.TestResultsFailed = &configpb.MetricThreshold{OneDay: proto.Int64(100)}
					lastPriority.Threshold.TestResultsFailed = &configpb.MetricThreshold{SevenDay: proto.Int64(100)}
					So(validate(cfg), ShouldBeNil)
				})

				Convey("seven day-filing threshold does not imply one-day keep open threshold", func() {
					// This implication does not work.
					bugFilingThres.TestResultsFailed = &configpb.MetricThreshold{SevenDay: proto.Int64(700)}
					lastPriority.Threshold.TestResultsFailed = &configpb.MetricThreshold{OneDay: proto.Int64(700)}
					So(validate(cfg), ShouldErrLike, "test_results_failed / seven_day): seven_day threshold must be set, with a value of at most 700")
				})

				Convey("metric threshold nil", func() {
					bugFilingThres.TestResultsFailed = &configpb.MetricThreshold{OneDay: proto.Int64(100)}
					lastPriority.Threshold.TestResultsFailed = nil
					So(validate(cfg), ShouldErrLike, "test_results_failed / one_day): one_day threshold must be set, with a value of at most 100")
				})

				Convey("metric threshold not set", func() {
					bugFilingThres.TestResultsFailed = &configpb.MetricThreshold{OneDay: proto.Int64(100)}
					lastPriority.Threshold.TestResultsFailed = &configpb.MetricThreshold{}
					So(validate(cfg), ShouldErrLike, "test_results_failed / one_day): one_day threshold must be set, with a value of at most 100")
				})
			})
			// Other thresholding validation cases tested under bug-filing threshold and are
			// not repeated given the implementation is shared.
		})

		Convey("priority hysteresis", func() {
			Convey("value too high", func() {
				cfg.Monorail.PriorityHysteresisPercent = 1001
				So(validate(cfg), ShouldErrLike, "value must not exceed 1000 percent")
			})
			Convey("value is negative", func() {
				cfg.Monorail.PriorityHysteresisPercent = -1
				So(validate(cfg), ShouldErrLike, "value must not be negative")
			})
		})

		Convey("monorail hostname", func() {
			// Only the domain name should be supplied, not the protocol.
			cfg.Monorail.MonorailHostname = "http://bugs.chromium.org"
			So(validate(cfg), ShouldErrLike, "invalid hostname")
		})

		Convey("display prefix", func() {
			// ";" is not allowed to appear in the prefix.
			cfg.Monorail.DisplayPrefix = "chromium:"
			So(validate(cfg), ShouldErrLike, "invalid display prefix")
		})
	})
	Convey("bug filing threshold", t, func() {
		cfg := CreatePlaceholderProjectConfig()
		threshold := cfg.BugFilingThreshold
		So(threshold, ShouldNotBeNil)

		Convey("must be specified", func() {
			cfg.BugFilingThreshold = nil
			So(validate(cfg), ShouldErrLike, "impact thresolds must be specified")
		})

		Convey("handles unset metric thresholds", func() {
			threshold := cfg.BugFilingThreshold
			threshold.CriticalFailuresExonerated = nil
			threshold.TestResultsFailed = nil
			threshold.TestRunsFailed = nil
			threshold.PresubmitRunsFailed = nil
			So(threshold, ShouldNotBeNil)
		})

		Convey("metric values are not negative", func() {
			// Test by threshold period.
			Convey("one day", func() {
				threshold.TestResultsFailed = &configpb.MetricThreshold{OneDay: proto.Int64(-1)}
				So(validate(cfg), ShouldErrLike, "value must be non-negative")
			})

			Convey("three day", func() {
				threshold.TestResultsFailed = &configpb.MetricThreshold{ThreeDay: proto.Int64(-1)}
				So(validate(cfg), ShouldErrLike, "value must be non-negative")
			})

			Convey("seven day", func() {
				threshold.TestResultsFailed = &configpb.MetricThreshold{SevenDay: proto.Int64(-1)}
				So(validate(cfg), ShouldErrLike, "value must be non-negative")
			})

			// Test by metric.
			Convey("critical test failures exonerated", func() {
				threshold.CriticalFailuresExonerated = &configpb.MetricThreshold{OneDay: proto.Int64(-1)}
				So(validate(cfg), ShouldErrLike, "value must be non-negative")
			})

			Convey("test results failed", func() {
				threshold.TestResultsFailed = &configpb.MetricThreshold{OneDay: proto.Int64(-1)}
				So(validate(cfg), ShouldErrLike, "value must be non-negative")
			})

			Convey("test runs failed", func() {
				threshold.TestRunsFailed = &configpb.MetricThreshold{OneDay: proto.Int64(-1)}
				So(validate(cfg), ShouldErrLike, "value must be non-negative")
			})

			Convey("presubmit runs failed", func() {
				threshold.PresubmitRunsFailed = &configpb.MetricThreshold{OneDay: proto.Int64(-1)}
				So(validate(cfg), ShouldErrLike, "value must be non-negative")
			})
		})
	})

	Convey("realm config", t, func() {
		cfg := CreatePlaceholderProjectConfig()
		So(len(cfg.Realms), ShouldEqual, 1)
		realm := cfg.Realms[0]

		Convey("realm name", func() {
			Convey("must be specified", func() {
				realm.Name = ""
				So(validate(cfg), ShouldErrLike, "empty realm_name is not allowed")
			})
			Convey("invalid", func() {
				realm.Name = "chromium:ci"
				So(validate(cfg), ShouldErrLike, `invalid realm_name: "chromium:ci"`)
			})
			Convey("valid", func() {
				realm.Name = "ci"
				So(validate(cfg), ShouldBeNil)
			})
		})

		Convey("TestVariantAnalysisConfig", func() {
			tvCfg := realm.TestVariantAnalysis
			So(tvCfg, ShouldNotBeNil)
			utCfg := tvCfg.UpdateTestVariantTask
			So(utCfg, ShouldNotBeNil)
			Convey("UpdateTestVariantTask", func() {
				Convey("interval", func() {
					Convey("empty not allowed", func() {
						utCfg.UpdateTestVariantTaskInterval = nil
						So(validate(cfg), ShouldErrLike, `empty interval is not allowed`)
					})
					Convey("must be greater than 0", func() {
						utCfg.UpdateTestVariantTaskInterval = durationpb.New(-time.Hour)
						So(validate(cfg), ShouldErrLike, `interval is less than 0`)
					})
				})

				Convey("duration", func() {
					Convey("empty not allowed", func() {
						utCfg.TestVariantStatusUpdateDuration = nil
						So(validate(cfg), ShouldErrLike, `empty duration is not allowed`)
					})
					Convey("must be greater than 0", func() {
						utCfg.TestVariantStatusUpdateDuration = durationpb.New(-time.Hour)
						So(validate(cfg), ShouldErrLike, `duration is less than 0`)
					})
				})
			})

			bqExports := tvCfg.BqExports
			So(len(bqExports), ShouldEqual, 1)
			bqe := bqExports[0]
			So(bqe, ShouldNotBeNil)
			Convey("BqExport", func() {
				table := bqe.Table
				So(table, ShouldNotBeNil)
				Convey("BigQueryTable", func() {
					Convey("cloud project", func() {
						Convey("should npt be empty", func() {
							table.CloudProject = ""
							So(validate(cfg), ShouldErrLike, "empty cloud_project is not allowed")
						})
						Convey("not end with hyphen", func() {
							table.CloudProject = "project-"
							So(validate(cfg), ShouldErrLike, `invalid cloud_project: "project-"`)
						})
						Convey("not too short", func() {
							table.CloudProject = "p"
							So(validate(cfg), ShouldErrLike, `invalid cloud_project: "p"`)
						})
						Convey("must start with letter", func() {
							table.CloudProject = "0project"
							So(validate(cfg), ShouldErrLike, `invalid cloud_project: "0project"`)
						})
					})

					Convey("dataset", func() {
						Convey("should not be empty", func() {
							table.Dataset = ""
							So(validate(cfg), ShouldErrLike, "empty dataset is not allowed")
						})
						Convey("should be valid", func() {
							table.Dataset = "data-set"
							So(validate(cfg), ShouldErrLike, `invalid dataset: "data-set"`)
						})
					})

					Convey("table", func() {
						Convey("should not be empty", func() {
							table.Table = ""
							So(validate(cfg), ShouldErrLike, "empty table_name is not allowed")
						})
						Convey("should be valid", func() {
							table.Table = "table/name"
							So(validate(cfg), ShouldErrLike, `invalid table_name: "table/name"`)
						})
					})
				})
			})
		})
	})

	Convey("clustering", t, func() {
		cfg := CreatePlaceholderProjectConfig()
		clustering := cfg.Clustering

		Convey("may not be specified", func() {
			cfg.Clustering = nil
			So(validate(cfg), ShouldBeNil)
		})
		Convey("rules must be valid", func() {
			rule := clustering.TestNameRules[0]
			Convey("name is not specified", func() {
				rule.Name = ""
				So(validate(cfg), ShouldErrLike, "empty name is not allowed")
			})
			Convey("name is invalid", func() {
				rule.Name = "<script>evil()</script>"
				So(validate(cfg), ShouldErrLike, "invalid name")
			})
			Convey("pattern is not specified", func() {
				rule.Pattern = ""
				// Make sure the like template does not refer to capture
				// groups in the pattern, to avoid other errors in this test.
				rule.LikeTemplate = "%blah%"
				So(validate(cfg), ShouldErrLike, "empty pattern is not allowed")
			})
			Convey("pattern is invalid", func() {
				rule.Pattern = "["
				So(validate(cfg), ShouldErrLike, `error parsing regexp: missing closing ]`)
			})
			Convey("like template is not specified", func() {
				rule.LikeTemplate = ""
				So(validate(cfg), ShouldErrLike, "empty like_template is not allowed")
			})
			Convey("like template is invalid", func() {
				rule.LikeTemplate = "blah${broken"
				So(validate(cfg), ShouldErrLike, `invalid use of the $ operator at position 4 in "blah${broken"`)
			})
		})
	})
}
