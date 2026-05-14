class Kagura < Formula
  desc "Terminal music player for Navidrome with a dancing DJ visualizer"
  homepage "https://github.com/Kiplol/kagura"
  url "https://github.com/Kiplol/kagura/archive/refs/tags/v0.1.1.tar.gz"
  sha256 "115d9e79485ae73e059fbf23b1ddaea619e8a94f0f123c7dbd03083235505edf"
  license "MIT"
  head "https://github.com/Kiplol/kagura.git", branch: "main"

  depends_on "go" => :build
  depends_on "mpv"

  def install
    system "go", "build", *std_go_args(ldflags: "-s -w"), "./cmd/kagura"
  end

  def caveats
    <<~EOS
      kagura requires mpv for audio playback (installed as a dependency).

      For automatic BPM detection from audio streams (optional):
        brew install aubio ffmpeg
    EOS
  end

  test do
    assert_predicate bin/"kagura", :exist?
  end
end
