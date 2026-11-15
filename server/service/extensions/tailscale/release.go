package tailscale

import installer "NanoKVM-Server/service/addons/install"

// The Tailscale release the add-on installs, pinned by version and SHA-256.
// It is not in the image: the owner installs it from the web UI, and the
// server downloads the static riscv64 package from pkgs.tailscale.com to
// /data, checks it, takes the two binaries out of it, each checked against
// its own sum, and packs them into the add-on's image (service/addons/install).
// Tailscale is BSD-3-Clause. A new release is a new pin here, its sum
// computed from a download and compared with the .sha256 the release server
// publishes beside the package, and the binaries' sums taken from the
// unpacked download. The web UI's manual install help
// (settings/tailscale/install-help.tsx) links the same package.

// Version is the pinned Tailscale release.
const Version = "1.104.7"

// release is a variable only so the tests can serve their own file; nothing
// else changes it. The package is 38,204,581 bytes, and the two binaries
// 68,802,021 together.
var release = installer.Manifest{
	Name:    "tailscale",
	Title:   "Tailscale",
	Version: Version,
	Licence: "BSD-3-Clause",
	Source:  "https://github.com/tailscale/tailscale/tree/v1.104.7",
	Artifact: installer.Artifact{
		URL:      "https://pkgs.tailscale.com/stable/tailscale_1.104.7_riscv64.tgz",
		SHA256:   "de8466ad6c96828671a2f57681da119293bac200597500d3fb30a7265e59bebb",
		Size:     38204581,
		Unpacked: 68802021,
		Max:      64 << 20,
		Format:   installer.FormatTarGz,
		Members: []installer.Member{
			{
				Path:   "tailscale_1.104.7_riscv64/tailscale",
				Name:   "tailscale",
				SHA256: "59e293a236713283976098e831b3178413637fd0a1e877fc5db6c72818a626e6",
				Mode:   0o755,
			},
			{
				Path:   "tailscale_1.104.7_riscv64/tailscaled",
				Name:   "tailscaled",
				SHA256: "06fd4e86b258442b148cf4d7f17e294b29fd27e19f9e80c4bc2a16ad773e2003",
				Mode:   0o755,
			},
		},
	},
}
