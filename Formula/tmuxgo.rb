# Homebrew formula template for tmuxgo.
# Publish by copying this file to a homebrew-tap repository (e.g.
# DKmiyan/homebrew-tap/Formula/tmuxgo.rb) and filling in the sha256 values
# from the release's checksums file.
class Tmuxgo < Formula
  desc "Lightweight tmux session navigator and organizer"
  homepage "https://github.com/DKmiyan/tmuxgo"
  version "0.1.0"
  license "MIT"
  depends_on "tmux"

  on_macos do
    if Hardware::CPU.arm?
      url "https://github.com/DKmiyan/tmuxgo/releases/download/v#{version}/tmuxgo_v#{version}_darwin_arm64.tar.gz"
      sha256 "FILL_FROM_CHECKSUMS_TXT"
    else
      url "https://github.com/DKmiyan/tmuxgo/releases/download/v#{version}/tmuxgo_v#{version}_darwin_amd64.tar.gz"
      sha256 "FILL_FROM_CHECKSUMS_TXT"
    end
  end

  on_linux do
    if Hardware::CPU.arm?
      url "https://github.com/DKmiyan/tmuxgo/releases/download/v#{version}/tmuxgo_v#{version}_linux_arm64.tar.gz"
      sha256 "FILL_FROM_CHECKSUMS_TXT"
    else
      url "https://github.com/DKmiyan/tmuxgo/releases/download/v#{version}/tmuxgo_v#{version}_linux_amd64.tar.gz"
      sha256 "FILL_FROM_CHECKSUMS_TXT"
    end
  end

  def install
    bin.install "tmuxgo"
  end

  test do
    assert_match "tmux session navigator", shell_output("#{bin}/tmuxgo --help")
  end
end
