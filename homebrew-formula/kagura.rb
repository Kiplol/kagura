class Kagura < Formula
  desc "Terminal music player for Navidrome with a dancing DJ visualizer"
  homepage "https://github.com/Kiplol/kagura"
  url "https://github.com/Kiplol/kagura/archive/refs/tags/v0.1.2.tar.gz"
  sha256 "8f24e6457fb88f58a14bc1fff2e911fb7e8ff467cb0d9f5172d090aed80078cc"
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
