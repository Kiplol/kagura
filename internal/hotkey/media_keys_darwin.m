// This file is compiled by CGo as part of the hotkey package on Darwin.
// Do NOT add a //go:build tag here — CGo picks up .m files automatically.

#import <MediaPlayer/MediaPlayer.h>
#import <Foundation/Foundation.h>

// Callback into Go — defined in media_keys_darwin.go with //export.
extern void goMediaKeyCallback(int cmd);

// Command constants must match the iota in daemon.go.
enum {
    CmdPlay            = 0,
    CmdPause           = 1,
    CmdTogglePlayPause = 2,
    CmdNext            = 3,
    CmdPrev            = 4,
    CmdVolumeUp        = 5,
    CmdVolumeDown      = 6,
    CmdToggleMute      = 7,
};

void setupRemoteCommands(void) {
    MPRemoteCommandCenter *cc = [MPRemoteCommandCenter sharedCommandCenter];

    [cc.playCommand addTargetWithHandler:^MPRemoteCommandHandlerStatus(MPRemoteCommandEvent *e) {
        goMediaKeyCallback(CmdPlay);
        return MPRemoteCommandHandlerStatusSuccess;
    }];

    [cc.pauseCommand addTargetWithHandler:^MPRemoteCommandHandlerStatus(MPRemoteCommandEvent *e) {
        goMediaKeyCallback(CmdPause);
        return MPRemoteCommandHandlerStatusSuccess;
    }];

    [cc.togglePlayPauseCommand addTargetWithHandler:^MPRemoteCommandHandlerStatus(MPRemoteCommandEvent *e) {
        goMediaKeyCallback(CmdTogglePlayPause);
        return MPRemoteCommandHandlerStatusSuccess;
    }];

    [cc.nextTrackCommand addTargetWithHandler:^MPRemoteCommandHandlerStatus(MPRemoteCommandEvent *e) {
        goMediaKeyCallback(CmdNext);
        return MPRemoteCommandHandlerStatusSuccess;
    }];

    [cc.previousTrackCommand addTargetWithHandler:^MPRemoteCommandHandlerStatus(MPRemoteCommandEvent *e) {
        goMediaKeyCallback(CmdPrev);
        return MPRemoteCommandHandlerStatusSuccess;
    }];

    // Enable seek support so Control Center shows a scrubber.
    [cc.changePlaybackPositionCommand setEnabled:YES];
    [cc.changePlaybackPositionCommand addTargetWithHandler:^MPRemoteCommandHandlerStatus(MPRemoteCommandEvent *e) {
        // Position changes handled by the seek command — no CmdSeek yet.
        return MPRemoteCommandHandlerStatusSuccess;
    }];
}

void teardownRemoteCommands(void) {
    MPRemoteCommandCenter *cc = [MPRemoteCommandCenter sharedCommandCenter];
    [cc.playCommand removeTarget:nil];
    [cc.pauseCommand removeTarget:nil];
    [cc.togglePlayPauseCommand removeTarget:nil];
    [cc.nextTrackCommand removeTarget:nil];
    [cc.previousTrackCommand removeTarget:nil];
    [cc.changePlaybackPositionCommand removeTarget:nil];
}

void updateNowPlayingInfo(const char *title, const char *artist,
                           double duration, double position, int playing) {
    NSMutableDictionary *info = [NSMutableDictionary dictionary];

    if (title && strlen(title) > 0)
        info[MPMediaItemPropertyTitle] = @(title);
    if (artist && strlen(artist) > 0)
        info[MPMediaItemPropertyArtist] = @(artist);
    if (duration > 0)
        info[MPMediaItemPropertyPlaybackDuration] = @(duration);

    info[MPNowPlayingInfoPropertyElapsedPlaybackTime] = @(position);
    info[MPNowPlayingInfoPropertyPlaybackRate]        = @(playing ? 1.0 : 0.0);
    info[MPNowPlayingInfoPropertyMediaType]           =
        @(MPNowPlayingInfoMediaTypeAudio);

    [MPNowPlayingInfoCenter defaultCenter].nowPlayingInfo = info;
}

// startObjCRunLoop spins an NSRunLoop on the calling thread.
// Called from a dedicated goroutine in Go so it never blocks the TUI.
void startObjCRunLoop(void) {
    [[NSRunLoop currentRunLoop] run];
}
