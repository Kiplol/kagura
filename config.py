"""Load and save user configuration (~/.config/navidrome-tui/config.json)."""

import json
from dataclasses import asdict, dataclass, field
from pathlib import Path

CONFIG_PATH = Path.home() / ".config" / "navidrome-tui" / "config.json"


@dataclass
class ServerConfig:
    url: str = ""
    username: str = ""
    password: str = ""


@dataclass
class HotkeyConfig:
    play_pause: str = ""
    next: str = ""
    prev: str = ""
    volume_up: str = ""
    volume_down: str = ""
    toggle_mute: str = ""


@dataclass
class Config:
    server: ServerConfig = field(default_factory=ServerConfig)
    hotkeys: HotkeyConfig = field(default_factory=HotkeyConfig)


def load() -> Config:
    if not CONFIG_PATH.exists():
        return Config()
    try:
        data = json.loads(CONFIG_PATH.read_text())
        server_data = data.get("server", {})
        hotkey_data = data.get("hotkeys", {})
        return Config(
            server=ServerConfig(
                url=server_data.get("url", ""),
                username=server_data.get("username", ""),
                password=server_data.get("password", ""),
            ),
            hotkeys=HotkeyConfig(
                play_pause=hotkey_data.get("play_pause", ""),
                next=hotkey_data.get("next", ""),
                prev=hotkey_data.get("prev", ""),
                volume_up=hotkey_data.get("volume_up", ""),
                volume_down=hotkey_data.get("volume_down", ""),
                toggle_mute=hotkey_data.get("toggle_mute", ""),
            ),
        )
    except Exception:
        return Config()


def save(cfg: Config) -> None:
    CONFIG_PATH.parent.mkdir(parents=True, exist_ok=True)
    CONFIG_PATH.write_text(
        json.dumps(
            {
                "server": asdict(cfg.server),
                "hotkeys": asdict(cfg.hotkeys),
            },
            indent=2,
        )
    )
