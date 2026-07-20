$version: "2.0"
$operationInputSuffix: "Input"
$operationOutputSuffix: "Output"

namespace example

use smithytranslate#contentType
use common#OperationErrors
use common#PagingParams
use common#CommonOutput
use smithy.api#idempotent

@readonly
@http(method: "GET", uri: "/api/v1/characters", code: 200)
@tags([
    "Character"
])
@auth([])
operation ListCharacters with [OperationErrors] {
    input := with [PagingParams] {
        /// Optional gender filter
        @httpQuery("gender")
        gender: String

        /// Optional ethnicity filter
        @httpQuery("ethnicity")
        ethnicity: String

        /// Optional age group filter
        @httpQuery("age_group")
        age_group: String

        /// Legacy single-tag filter; prefer repeating `tags` for AND semantics (Director).
        @httpQuery("tag")
        tag: String

        /// `system` = platform presets (default). `user-upload` = current user's self-upload digital humans (requires Medeo-User-Id).
        @httpQuery("scope")
        scope: CharacterScope

        /// When `scope=user-upload`, filter by Medeo project id.
        @httpQuery("project_id")
        project_id: String

        /// AND tag filter: repeat query key `tags`, e.g. `?tags=country:Italy&tags=occupation:miner`
        @httpQuery("tags")
        tags: StringList
    }
    output := with [CommonOutput] {
        @httpPayload
        @required
        @contentType("application/json")
        body: ListCharactersResp
    }
}

@readonly
@http(method: "GET", uri: "/api/v1/characters/{character_id}", code: 200)
@tags([
    "Character"
])
operation GetCharacter with [OperationErrors] {
    input := {
        @httpLabel
        @required
        character_id: String
    }
    output := with [CommonOutput] {
        @httpPayload
        @required
        @contentType("application/json")
        body: Character
    }
}

@http(method: "POST", uri: "/api/v1/characters", code: 200)
@tags([
    "Character"
])
operation CreateCharacter with [OperationErrors] {
    input := {
        @httpPayload
        @required
        @contentType("application/json")
        body: CreateCharacterReq
    }
    output := with [CommonOutput] {
        @httpPayload
        @required
        @contentType("application/json")
        body: Character
    }
}

@idempotent
@http(method: "PUT", uri: "/api/v1/characters/{character_id}", code: 200)
@tags([
    "Character"
])
operation UpdateCharacter with [OperationErrors] {
    input := {
        @httpLabel
        @required
        character_id: String

        @httpPayload
        @required
        @contentType("application/json")
        body: UpdateCharacterReq
    }
    output := with [CommonOutput] {
        @httpPayload
        @required
        @contentType("application/json")
        body: Character
    }
}

@idempotent
@http(method: "DELETE", uri: "/api/v1/characters/{character_id}", code: 200)
@tags([
    "Character"
])
operation DeleteCharacter with [OperationErrors] {
    input := {
        @httpLabel
        @required
        character_id: String
    }
}
