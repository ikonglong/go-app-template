$version: "2.0"

namespace example

use common#PageMetadata


structure Character {
    id: String

    display_name: String

    description: String

    scope: CharacterScope

    gender: Gender

    thumbnail_image_id: String

    thumbnail_storage_key: String

    ref_image_ids: StringList

    ethnicity: String

    age_group: String

    tags: StringList

    sort_order: Integer

    @timestampFormat("date-time")
    created_at: Timestamp

    @timestampFormat("date-time")
    updated_at: Timestamp

    /// Third-party asset id when registered (e.g. after CreateAsset); may be absent.
    third_party_id: String

    /// Set when the character row was written after third-party asset registration reached `Active`
    /// (e.g. user `POST /api/v2/characters` or internal `POST /internal/v2/characters` with thumbnail_registration).
    /// Omitted on list/get and other responses.
    third_party_asset: ThirdPartyAssetSnapshot
}

/// Third-party asset outcome surfaced on create success when registration was awaited.
structure ThirdPartyAssetSnapshot {
    /// `Active` when the character row was written after GetAsset confirmed availability.
    @required
    status: String
}

enum CharacterScope {
    USER = "user"
    SYSTEM = "system"
    /// Self-upload digital human; persisted from `POST /api/v2/characters`; list with `scope=user-upload`.
    USER_UPLOAD = "user-upload"
}

enum Gender {
    MALE = "male"
    FEMALE = "female"
    NEUTRAL = "neutral"
}

/// Request body for `POST /api/v2/characters` (current user's custom character).
structure CreateCharacterReq {
    @required
    display_name: String

    /// Optional; if omitted, server stores `display_name` as description.
    description: String

    @required
    thumbnail_image_id: String

    /// When set, ties ownership to a Medeo project for scoped listing.
    project_id: String

    gender: Gender

    ref_image_ids: StringList

    ethnicity: String

    age_group: String

    tags: StringList
}

structure UpdateCharacterReq {
    display_name: String

    description: String

    gender: Gender

    thumbnail_image_id: String

    ref_image_ids: StringList

    ethnicity: String

    age_group: String

    tags: StringList

    sort_order: Integer

    third_party_id: String
}

structure ListCharactersResp with [PageMetadata] {
    @required
    list: CharacterList
}

list CharacterList {
    member: Character
}

list StringList {
    member: String
}
