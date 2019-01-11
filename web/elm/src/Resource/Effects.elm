module Resource.Effects exposing (Effect(..), runEffect)

import Concourse
import Concourse.Pagination exposing (Page)
import Concourse.Resource
import Effects exposing (setTitle)
import LoginRedirect
import Navigation
import Resource.Msgs exposing (Msg(..), VersionToggleAction(..))
import Task


type Effect
    = FetchResource Concourse.ResourceIdentifier
    | FetchVersionedResources Concourse.ResourceIdentifier (Maybe Page)
    | SetTitle String
    | RedirectToLogin
    | FetchInputTo Concourse.VersionedResourceIdentifier
    | FetchOutputOf Concourse.VersionedResourceIdentifier
    | NavigateTo String
    | DoPinVersion Concourse.VersionedResourceIdentifier Concourse.CSRFToken
    | DoUnpinVersion Concourse.ResourceIdentifier Concourse.CSRFToken
    | DoEnableDisableVersionedResource VersionToggleAction Concourse.VersionedResourceIdentifier Concourse.CSRFToken


runEffect : Effect -> Cmd Msg
runEffect effect =
    case effect of
        FetchResource id ->
            fetchResource id

        FetchVersionedResources id paging ->
            fetchVersionedResources id paging

        SetTitle newTitle ->
            setTitle newTitle

        RedirectToLogin ->
            LoginRedirect.requestLoginRedirect ""

        FetchInputTo id ->
            fetchInputTo id

        FetchOutputOf id ->
            fetchOutputOf id

        NavigateTo newUrl ->
            Navigation.newUrl newUrl

        DoPinVersion version csrfToken ->
            Task.attempt VersionPinned <|
                Concourse.Resource.pinVersion version csrfToken

        DoUnpinVersion id csrfToken ->
            Task.attempt VersionUnpinned <|
                Concourse.Resource.unpinVersion id csrfToken

        DoEnableDisableVersionedResource action id csrfToken ->
            Task.attempt (VersionToggled action id.versionID) <|
                Concourse.Resource.enableDisableVersionedResource
                    (action == Enable)
                    id
                    csrfToken


fetchResource : Concourse.ResourceIdentifier -> Cmd Msg
fetchResource resourceIdentifier =
    Task.attempt ResourceFetched <|
        Concourse.Resource.fetchResource resourceIdentifier


fetchVersionedResources : Concourse.ResourceIdentifier -> Maybe Page -> Cmd Msg
fetchVersionedResources resourceIdentifier page =
    Task.attempt (VersionedResourcesFetched page) <|
        Concourse.Resource.fetchVersionedResources resourceIdentifier page


fetchInputTo : Concourse.VersionedResourceIdentifier -> Cmd Msg
fetchInputTo versionedResourceIdentifier =
    Task.attempt (InputToFetched versionedResourceIdentifier.versionID) <|
        Concourse.Resource.fetchInputTo versionedResourceIdentifier


fetchOutputOf : Concourse.VersionedResourceIdentifier -> Cmd Msg
fetchOutputOf versionedResourceIdentifier =
    Task.attempt (OutputOfFetched versionedResourceIdentifier.versionID) <|
        Concourse.Resource.fetchOutputOf versionedResourceIdentifier
