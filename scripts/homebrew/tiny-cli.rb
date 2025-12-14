class TinyCli < Formula
  desc "Terminal UI client for Tiny URL Shortener"
  homepage "https://github.com/Varun5711/shorternit"
  version "1.0.0"  
  license "MIT"

  on_macos do
    if Hardware::CPU.arm?
      url "https://github.com/Varun5711/shorternit/releases/download/v#{version}/tiny-cli-darwin-arm64"
      sha256 "PLACEHOLDER_SHA256_DARWIN_ARM64" 

      def install
        bin.install "tiny-cli-darwin-arm64" => "tiny-cli"
      end
    else
      url "https://github.com/Varun5711/shorternit/releases/download/v#{version}/tiny-cli-darwin-amd64"
      sha256 "PLACEHOLDER_SHA256_DARWIN_AMD64"

      def install
        bin.install "tiny-cli-darwin-amd64" => "tiny-cli"
      end
    end
  end

  on_linux do
    if Hardware::CPU.arm?
      url "https://github.com/Varun5711/shorternit/releases/download/v#{version}/tiny-cli-linux-arm64"
      sha256 "PLACEHOLDER_SHA256_LINUX_ARM64" 

      def install
        bin.install "tiny-cli-linux-arm64" => "tiny-cli"
      end
    else
      url "https://github.com/Varun5711/shorternit/releases/download/v#{version}/tiny-cli-linux-amd64"
      sha256 "PLACEHOLDER_SHA256_LINUX_AMD64" 

      def install
        bin.install "tiny-cli-linux-amd64" => "tiny-cli"
      end
    end
  end

  test do
    assert_match "tiny-cli", shell_output("#{bin}/tiny-cli --version")
  end
end
