class Sophia < Formula
  desc "Intent-first workflow over Git for AI-assisted code changes"
  homepage "https://github.com/ithena-labs/sophia"
  version "0.1.0"
  license "MIT"

  if OS.mac? && Hardware::CPU.arm?
    url "https://github.com/ithena-labs/sophia/releases/download/v#{version}/sophia_v#{version}_darwin_arm64.tar.gz"
    sha256 "REPLACE_WITH_SHA256_DARWIN_ARM64"
  elsif OS.mac? && Hardware::CPU.intel?
    url "https://github.com/ithena-labs/sophia/releases/download/v#{version}/sophia_v#{version}_darwin_amd64.tar.gz"
    sha256 "REPLACE_WITH_SHA256_DARWIN_AMD64"
  elsif OS.linux? && Hardware::CPU.arm?
    url "https://github.com/ithena-labs/sophia/releases/download/v#{version}/sophia_v#{version}_linux_arm64.tar.gz"
    sha256 "REPLACE_WITH_SHA256_LINUX_ARM64"
  else
    url "https://github.com/ithena-labs/sophia/releases/download/v#{version}/sophia_v#{version}_linux_amd64.tar.gz"
    sha256 "REPLACE_WITH_SHA256_LINUX_AMD64"
  end

  def install
    bin.install "sophia"
  end

  test do
    output = shell_output("#{bin}/sophia version")
    assert_match "version:", output
    assert_match "commit:", output
    assert_match "build_date:", output
  end
end
