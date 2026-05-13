"""Subsonic REST API client (token auth, JSON responses)."""

import hashlib
import random
import string
from dataclasses import dataclass, field
from typing import Optional
from urllib.parse import urlencode

import requests


@dataclass
class Artist:
    id: str
    name: str
    album_count: int = 0


@dataclass
class Album:
    id: str
    name: str
    artist: str = ""
    artist_id: str = ""
    year: int = 0
    song_count: int = 0


@dataclass
class Song:
    id: str
    title: str
    artist: str = ""
    album: str = ""
    album_id: str = ""
    track: int = 0
    duration: int = 0
    bit_rate: int = 0
    suffix: str = ""


@dataclass
class Playlist:
    id: str
    name: str
    song_count: int = 0
    duration: int = 0


def _fmt_dur(secs: int) -> str:
    m, s = divmod(secs, 60)
    return f"{m}:{s:02d}"


class SubsonicClient:
    def __init__(self, url: str, username: str, password: str) -> None:
        self.base_url = url.rstrip("/")
        self.username = username
        self.password = password
        self._session = requests.Session()

    # ------------------------------------------------------------------
    # Auth helpers
    # ------------------------------------------------------------------

    def _auth(self) -> dict:
        salt = "".join(random.choices(string.ascii_lowercase + string.digits, k=6))
        token = hashlib.md5((self.password + salt).encode()).hexdigest()
        return {
            "u": self.username,
            "t": token,
            "s": salt,
            "v": "1.16.1",
            "c": "navidrome-tui",
            "f": "json",
        }

    def _get(self, endpoint: str, **params) -> dict:
        resp = self._session.get(
            f"{self.base_url}/rest/{endpoint}",
            params={**self._auth(), **params},
            timeout=15,
        )
        resp.raise_for_status()
        body = resp.json()["subsonic-response"]
        if body["status"] != "ok":
            msg = body.get("error", {}).get("message", "Unknown error")
            raise RuntimeError(msg)
        return body

    # ------------------------------------------------------------------
    # API methods
    # ------------------------------------------------------------------

    def ping(self) -> bool:
        try:
            self._get("ping")
            return True
        except Exception:
            return False

    def get_artists(self) -> list[Artist]:
        data = self._get("getArtists")
        artists: list[Artist] = []
        for index in data.get("artists", {}).get("index", []):
            for a in index.get("artist", []):
                artists.append(
                    Artist(
                        id=str(a["id"]),
                        name=a["name"],
                        album_count=int(a.get("albumCount", 0)),
                    )
                )
        return sorted(artists, key=lambda a: a.name.casefold())

    def get_albums(self, artist_id: str) -> list[Album]:
        data = self._get("getArtist", id=artist_id)
        albums: list[Album] = []
        for a in data.get("artist", {}).get("album", []):
            albums.append(
                Album(
                    id=str(a["id"]),
                    name=a["name"],
                    artist=a.get("artist", ""),
                    artist_id=str(a.get("artistId", "")),
                    year=int(a.get("year", 0)),
                    song_count=int(a.get("songCount", 0)),
                )
            )
        return sorted(albums, key=lambda a: a.year)

    def get_songs(self, album_id: str) -> list[Song]:
        data = self._get("getAlbum", id=album_id)
        songs: list[Song] = []
        for s in data.get("album", {}).get("song", []):
            songs.append(
                Song(
                    id=str(s["id"]),
                    title=s["title"],
                    artist=s.get("artist", ""),
                    album=s.get("album", ""),
                    album_id=str(s.get("albumId", "")),
                    track=int(s.get("track", 0)),
                    duration=int(s.get("duration", 0)),
                    bit_rate=int(s.get("bitRate", 0)),
                    suffix=s.get("suffix", ""),
                )
            )
        return sorted(songs, key=lambda s: s.track)

    def get_playlists(self) -> list[Playlist]:
        data = self._get("getPlaylists")
        playlists: list[Playlist] = []
        for p in data.get("playlists", {}).get("playlist", []):
            playlists.append(
                Playlist(
                    id=str(p["id"]),
                    name=p["name"],
                    song_count=int(p.get("songCount", 0)),
                    duration=int(p.get("duration", 0)),
                )
            )
        return playlists

    def get_playlist_songs(self, playlist_id: str) -> list[Song]:
        data = self._get("getPlaylist", id=playlist_id)
        songs: list[Song] = []
        for s in data.get("playlist", {}).get("entry", []):
            songs.append(
                Song(
                    id=str(s["id"]),
                    title=s["title"],
                    artist=s.get("artist", ""),
                    album=s.get("album", ""),
                    track=int(s.get("track", 0)),
                    duration=int(s.get("duration", 0)),
                    bit_rate=int(s.get("bitRate", 0)),
                    suffix=s.get("suffix", ""),
                )
            )
        return songs

    def search(
        self, query: str
    ) -> tuple[list[Artist], list[Album], list[Song]]:
        data = self._get(
            "search3",
            query=query,
            artistCount=10,
            albumCount=10,
            songCount=20,
        )
        result = data.get("searchResult3", {})

        artists = [
            Artist(id=str(a["id"]), name=a["name"], album_count=a.get("albumCount", 0))
            for a in result.get("artist", [])
        ]
        albums = [
            Album(
                id=str(a["id"]),
                name=a["name"],
                artist=a.get("artist", ""),
                year=int(a.get("year", 0)),
            )
            for a in result.get("album", [])
        ]
        songs = [
            Song(
                id=str(s["id"]),
                title=s["title"],
                artist=s.get("artist", ""),
                album=s.get("album", ""),
                duration=int(s.get("duration", 0)),
            )
            for s in result.get("song", [])
        ]
        return artists, albums, songs

    def get_all_albums(self, size: int = 500) -> list[Album]:
        """Return all albums sorted alphabetically (uses getAlbumList2)."""
        data = self._get("getAlbumList2", type="alphabeticalByName", size=size)
        albums: list[Album] = []
        for a in data.get("albumList2", {}).get("album", []):
            albums.append(
                Album(
                    id=str(a["id"]),
                    name=a["name"],
                    artist=a.get("artist", ""),
                    artist_id=str(a.get("artistId", "")),
                    year=int(a.get("year", 0)),
                    song_count=int(a.get("songCount", 0)),
                )
            )
        return albums

    def get_random_songs(self, size: int = 50) -> list[Song]:
        """Return a random selection of songs."""
        data = self._get("getRandomSongs", size=size)
        songs: list[Song] = []
        for s in data.get("randomSongs", {}).get("song", []):
            songs.append(
                Song(
                    id=str(s["id"]),
                    title=s["title"],
                    artist=s.get("artist", ""),
                    album=s.get("album", ""),
                    duration=int(s.get("duration", 0)),
                    bit_rate=int(s.get("bitRate", 0)),
                    suffix=s.get("suffix", ""),
                )
            )
        return songs

    def stream_url(self, song_id: str) -> str:
        params = {**self._auth(), "id": song_id}
        return f"{self.base_url}/rest/stream?{urlencode(params)}"
