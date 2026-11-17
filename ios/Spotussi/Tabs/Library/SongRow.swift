import SpotussiKit
import SwiftUI

/// One saved song or playlist song, as the web's song-row: artwork, name
/// and artists, when it was saved or added and what became of it, and its
/// plays as rotation lamps over the count. In a playlist, its place leads
/// and the plays started from the playlist follow. Not a button: whoever
/// lists it decides where it leads.
struct SongRow: View {
    let song: SavedSong
    /// In a list of never-played songs only, where saying so would repeat.
    var quiet = false
    var zoom: ZoomOrigin?
    @Environment(\.dynamicTypeSize) private var dynamicTypeSize
    @ScaledMetric(relativeTo: .caption) private var positionWidth = 24

    var body: some View {
        let layout =
            dynamicTypeSize.isAccessibilitySize
            ? AnyLayout(VStackLayout(alignment: .leading, spacing: 8))
            : AnyLayout(HStackLayout(alignment: .center, spacing: 12))
        let now = Date.now
        layout {
            HStack(spacing: 12) {
                if let position = song.position {
                    Text(position + 1, format: .number)
                        .font(.caption.monospacedDigit())
                        .foregroundStyle(.secondary)
                        .frame(minWidth: positionWidth, alignment: .trailing)
                }
                ArtworkImage(url: song.imageURL, side: 40, symbol: "music.note")
                    .zoomSource(zoom)
            }
            VStack(alignment: .leading, spacing: 2) {
                Text(song.track.name)
                    .font(.body.weight(.medium))
                if let artists = song.artists {
                    Text(artists)
                        .font(.caption)
                        .foregroundStyle(.secondary)
                }
                if let details = details(now: now) {
                    details
                        .font(.caption.monospacedDigit())
                        .foregroundStyle(.secondary)
                }
            }
            .frame(maxWidth: .infinity, alignment: .leading)
            VStack(alignment: dynamicTypeSize.isAccessibilitySize ? .leading : .trailing, spacing: 4) {
                RotationLamps(song: song)
                Text("\(song.plays) plays")
                    .font(.caption.monospacedDigit())
                if let here = song.playsHere {
                    Text("\(here) here")
                        .font(.caption2.monospacedDigit())
                        .foregroundStyle(.secondary)
                }
            }
            .fixedSize()
        }
        .padding(.vertical, 6)
        .frame(minHeight: 44)
        .contentShape(.rect)
        .accessibilityElement(children: .ignore)
        .accessibilityLabel(Self.spokenLabel(song))
        .accessibilityValue(Self.spokenValue(song, quiet: quiet, now: now))
    }

    /// "Saved Oct 14 · Never played", the note in full strength when it
    /// says the song was never played.
    private func details(now: Date) -> Text? {
        let added = Self.added(song, now: now).map { Text($0) }
        let note = song.note(quiet: quiet).map { note in
            note == .neverPlayed
                ? Text(Self.words(note, now: now)).foregroundStyle(.primary)
                : Text(Self.words(note, now: now))
        }
        switch (added, note) {
        case let (added?, note?): return Text("\(added) · \(note)")
        case let (added?, nil): return added
        case let (nil, note?): return note
        case (nil, nil): return nil
        }
    }

    // MARK: - Words

    /// "Saved Oct 14", "You added Oct 14", "jo added Oct 14".
    static func added(_ song: SavedSong, now: Date) -> String? {
        let date = song.addedAt.map { Figures.date($0, now: now) }
        switch (song.adder, date) {
        case (.you, let date?): return String(localized: "You added \(date)")
        case (.you, nil): return String(localized: "You added")
        case (.other(let name), let date?): return String(localized: "\(name) added \(date)")
        case (.other(let name), nil): return String(localized: "\(name) added")
        case (nil, let date?):
            return song.position == nil ? String(localized: "Saved \(date)") : String(localized: "Added \(date)")
        case (nil, nil): return nil
        }
    }

    static func words(_ note: SavedSong.Note, now: Date) -> String {
        switch note {
        case .neverPlayed: String(localized: "Never played")
        case .otherRelease: String(localized: "Other release played")
        case .rotation(nil): String(localized: "In rotation")
        case .rotation(.beforeSaving): String(localized: "In rotation before saving")
        case .rotation(.sameDay): String(localized: "In rotation the day it was saved")
        case .rotation(.days(let days)): String(localized: "In rotation \(days) days after saving")
        case .lastPlayed(let date): String(localized: "Last played \(Figures.date(date, now: now))")
        }
    }

    /// "The Tangled Signal, Ezra Quiveto"
    static func spokenLabel(_ song: SavedSong) -> String {
        let name = song.position.map { String(localized: "\($0 + 1). \(song.track.name)") } ?? song.track.name
        return [name, song.artists].compactMap(\.self).joined(separator: ", ")
    }

    /// "17 plays, Saved Oct 14, In rotation before saving, 3 from this playlist"
    static func spokenValue(_ song: SavedSong, quiet: Bool, now: Date) -> String {
        var parts = [Spoken.plays(song.plays)]
        if let here = song.playsHere {
            parts.append(String(localized: "\(here) from this playlist"))
        }
        if let added = added(song, now: now) { parts.append(added) }
        if let note = song.note(quiet: quiet) { parts.append(words(note, now: now)) }
        return parts.joined(separator: ", ")
    }
}
