$version: "2.0"

namespace example

use aws.protocols#restJson1

@restJson1
// Model authentication with Smithy's predefined auth traits:
// https://smithy.io/2.0/spec/authentication-traits.html
// Scalar Docs does not render authentication headers as operation inputs, so
// keep authentication on the service/operation rather than modeling an
// Authorization header in every input shape.
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
