// Copyright 2022 syzkaller project authors. All rights reserved.
// Use of this source code is governed by Apache 2 LICENSE that can be found in the LICENSE file.

package main

import (
	"fmt"
	"sort"
	"testing"
	"time"

	"github.com/google/syzkaller/dashboard/dashapi"
	"github.com/google/syzkaller/pkg/email"
)

func TestBuildAssetLifetime(t *testing.T) {
	c := NewCtx(t)
	defer c.Close()

	build := testBuild(1)
	// Embed one of the assets right away.
	build.Assets = []dashapi.NewAsset{
		{
			Type:        dashapi.KernelObject,
			DownloadURL: "http://google.com/vmlinux",
		},
	}
	c.client2.UploadBuild(build)

	// "Upload" several more assets.
	c.expectOK(c.client2.AddBuildAssets(&dashapi.AddBuildAssetsReq{
		BuildID: build.ID,
		Assets: []dashapi.NewAsset{
			{
				Type:        dashapi.BootableDisk,
				DownloadURL: "http://google.com/bootable_disk",
			},
		},
	}))
	c.expectOK(c.client2.AddBuildAssets(&dashapi.AddBuildAssetsReq{
		BuildID: build.ID,
		Assets: []dashapi.NewAsset{
			{
				Type:        dashapi.HTMLCoverageReport,
				DownloadURL: "http://google.com/coverage.html",
			},
		},
	}))

	crash := testCrash(build, 1)
	crash.Maintainers = []string{`"Foo Bar" <foo@bar.com>`, `bar@foo.com`, `idont@want.EMAILS`}
	c.client2.ReportCrash(crash)

	// Test that the reporting email is correct.
	msg := c.pollEmailBug()
	sender, extBugID, err := email.RemoveAddrContext(msg.Sender)
	c.expectOK(err)
	_, dbCrash, dbBuild := c.loadBug(extBugID)
	crashLogLink := externalLink(c.ctx, textCrashLog, dbCrash.Log)
	kernelConfigLink := externalLink(c.ctx, textKernelConfig, dbBuild.KernelConfig)
	c.expectEQ(sender, fromAddr(c.ctx))
	to := config.Namespaces["test2"].Reporting[0].Config.(*EmailConfig).Email
	c.expectEQ(msg.To, []string{to})
	c.expectEQ(msg.Subject, crash.Title)
	c.expectEQ(len(msg.Attachments), 0)
	c.expectEQ(msg.Body, fmt.Sprintf(`Hello,

syzbot found the following issue on:

HEAD commit:    111111111111 kernel_commit_title1
git tree:       repo1 branch1
console output: %[2]v
kernel config:  %[3]v
dashboard link: https://testapp.appspot.com/bug?extid=%[1]v
compiler:       compiler1
CC:             [bar@foo.com foo@bar.com idont@want.EMAILS]

Unfortunately, I don't have any reproducer for this issue yet.

Downloadable assets:
disk image: http://google.com/bootable_disk
vmlinux: http://google.com/vmlinux

IMPORTANT: if you fix the issue, please add the following tag to the commit:
Reported-by: syzbot+%[1]v@testapp.appspotmail.com

report1

---
This report is generated by a bot. It may contain errors.
See https://goo.gl/tpsmEJ for more information about syzbot.
syzbot engineers can be reached at syzkaller@googlegroups.com.

syzbot will keep track of this issue. See:
https://goo.gl/tpsmEJ#status for how to communicate with syzbot.`,
		extBugID, crashLogLink, kernelConfigLink))
	c.checkURLContents(crashLogLink, crash.Log)
	c.checkURLContents(kernelConfigLink, build.KernelConfig)

	// We query the needed assets. We need all 3.
	needed, err := c.client2.NeededAssetsList()
	c.expectOK(err)
	sort.Strings(needed.DownloadURLs)
	allDownloadURLs := []string{
		"http://google.com/bootable_disk",
		"http://google.com/coverage.html",
		"http://google.com/vmlinux",
	}
	c.expectEQ(needed.DownloadURLs, allDownloadURLs)

	// Invalidate the bug.
	c.client.updateBug(extBugID, dashapi.BugStatusInvalid, "")
	_, err = c.GET("/deprecate_assets")
	c.expectOK(err)

	// Query the needed assets once more, so far there should be no change.
	needed, err = c.client2.NeededAssetsList()
	c.expectOK(err)
	sort.Strings(needed.DownloadURLs)
	c.expectEQ(needed.DownloadURLs, allDownloadURLs)

	// Skip one month and deprecate assets.
	c.advanceTime(time.Hour * 24 * 31)
	_, err = c.GET("/deprecate_assets")
	c.expectOK(err)

	// Only the html asset should have persisted.
	needed, err = c.client2.NeededAssetsList()
	c.expectOK(err)
	c.expectEQ(needed.DownloadURLs, []string{"http://google.com/coverage.html"})
}

func TestCoverReportDisplay(t *testing.T) {
	c := NewCtx(t)
	defer c.Close()

	build := testBuild(1)
	c.client.UploadBuild(build)

	// Upload the second build to just make sure coverage reports are assigned per-manager.
	c.client.UploadBuild(testBuild(2))

	// We expect no coverage reports to be present.
	uiManagers, err := loadManagers(c.ctx, AccessAdmin, "test1", "")
	c.expectOK(err)
	c.expectEQ(len(uiManagers), 2)
	c.expectEQ(uiManagers[0].CoverLink, "")
	c.expectEQ(uiManagers[1].CoverLink, "")

	// Upload an asset.
	origHTMLAsset := "http://google.com/coverage0.html"
	c.expectOK(c.client.AddBuildAssets(&dashapi.AddBuildAssetsReq{
		BuildID: build.ID,
		Assets: []dashapi.NewAsset{
			{
				Type:        dashapi.HTMLCoverageReport,
				DownloadURL: origHTMLAsset,
			},
		},
	}))
	uiManagers, err = loadManagers(c.ctx, AccessAdmin, "test1", "")
	c.expectOK(err)
	c.expectEQ(len(uiManagers), 2)
	c.expectEQ(uiManagers[0].CoverLink, origHTMLAsset)
	c.expectEQ(uiManagers[1].CoverLink, "")

	// Upload a newer coverage.
	newHTMLAsset := "http://google.com/coverage1.html"
	c.expectOK(c.client.AddBuildAssets(&dashapi.AddBuildAssetsReq{
		BuildID: build.ID,
		Assets: []dashapi.NewAsset{
			{
				Type:        dashapi.HTMLCoverageReport,
				DownloadURL: newHTMLAsset,
			},
		},
	}))
	uiManagers, err = loadManagers(c.ctx, AccessAdmin, "test1", "")
	c.expectOK(err)
	c.expectEQ(len(uiManagers), 2)
	c.expectEQ(uiManagers[0].CoverLink, newHTMLAsset)
	c.expectEQ(uiManagers[1].CoverLink, "")
}

func TestCoverReportDeprecation(t *testing.T) {
	c := NewCtx(t)
	defer c.Close()

	ensureNeeded := func(needed []string) {
		_, err := c.GET("/deprecate_assets")
		c.expectOK(err)
		neededResp, err := c.client.NeededAssetsList()
		c.expectOK(err)
		sort.Strings(neededResp.DownloadURLs)
		sort.Strings(needed)
		c.expectEQ(neededResp.DownloadURLs, needed)
	}

	build := testBuild(1)
	c.client.UploadBuild(build)

	uploadReport := func(url string) {
		c.expectOK(c.client.AddBuildAssets(&dashapi.AddBuildAssetsReq{
			BuildID: build.ID,
			Assets: []dashapi.NewAsset{
				{
					Type:        dashapi.HTMLCoverageReport,
					DownloadURL: url,
				},
			},
		}))
	}

	// Week 1. Saturday Jan 1st, 2000.
	weekOneFirst := "http://google.com/coverage1_1.html"
	uploadReport(weekOneFirst)

	// Week 1. Sunday Jan 2nd, 2000.
	weekOneSecond := "http://google.com/coverage1_2.html"
	c.advanceTime(time.Hour * 24)
	uploadReport(weekOneSecond)
	ensureNeeded([]string{weekOneFirst, weekOneSecond})

	// Week 2. Tuesday Jan 4nd, 2000.
	weekTwoFirst := "http://google.com/coverage2_1.html"
	c.advanceTime(time.Hour * 24 * 2)
	uploadReport(weekTwoFirst)
	ensureNeeded([]string{weekOneFirst, weekOneSecond, weekTwoFirst})

	// Week 2. Thu Jan 6nd, 2000.
	weekTwoSecond := "http://google.com/coverage2_2.html"
	c.advanceTime(time.Hour * 24 * 2)
	uploadReport(weekTwoSecond)
	ensureNeeded([]string{weekOneFirst, weekOneSecond, weekTwoFirst, weekTwoSecond})

	// Week 3. Monday Jan 10th, 2000.
	weekThreeFirst := "http://google.com/coverage3_1.html"
	c.advanceTime(time.Hour * 24 * 4)
	uploadReport(weekThreeFirst)
	ensureNeeded([]string{weekOneFirst, weekOneSecond, weekTwoFirst, weekTwoSecond, weekThreeFirst})

	// Week 4. Monday Jan 17th, 2000.
	weekFourFirst := "http://google.com/coverage4_1.html"
	c.advanceTime(time.Hour * 24 * 7)
	uploadReport(weekFourFirst)

	t.Logf("embargo is over, time is %s", timeNow(c.ctx))
	// Note that now that the two week deletion embargo has passed, the first asset
	// begins to falls out.
	ensureNeeded([]string{weekOneSecond, weekTwoFirst, weekTwoSecond, weekThreeFirst, weekFourFirst})

	// Week 5. Monday Jan 24th, 2000.
	c.advanceTime(time.Hour * 24 * 7)
	ensureNeeded([]string{weekOneSecond, weekTwoSecond, weekThreeFirst, weekFourFirst})

	// A year later.
	c.advanceTime(time.Hour * 24 * 365)
	ensureNeeded([]string{weekOneSecond, weekTwoSecond, weekThreeFirst, weekFourFirst})
}