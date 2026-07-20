$version: "2.0"

namespace example

use aws.protocols#restJson1

@restJson1
@httpBearerAuth
service APIService {
    version: "1.0.0"
    operations: [
        // Character
        ListCharacters
        GetCharacter
        CreateCharacter
        UpdateCharacter
        DeleteCharacter
    ]
}
