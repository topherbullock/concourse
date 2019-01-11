module Resource.Msgs exposing
    ( Hoverable(..)
    , Msg(..)
    , VersionToggleAction(..)
    )

import Concourse
import Concourse.Pagination exposing (Page, Paginated)
import Http
import Time exposing (Time)


type VersionToggleAction
    = Enable
    | Disable


type Hoverable
    = PreviousPage
    | NextPage
    | None


type Msg
    = Noop
    | AutoupdateTimerTicked Time
    | ResourceFetched (Result Http.Error Concourse.Resource)
    | VersionedResourcesFetched (Maybe Page) (Result Http.Error (Paginated Concourse.VersionedResource))
    | LoadPage Page
    | ClockTick Time.Time
    | ExpandVersionedResource Int
    | InputToFetched Int (Result Http.Error (List Concourse.Build))
    | OutputOfFetched Int (Result Http.Error (List Concourse.Build))
    | NavTo String
    | TogglePinBarTooltip
    | ToggleVersionTooltip
    | PinVersion Int
    | UnpinVersion
    | VersionPinned (Result Http.Error ())
    | VersionUnpinned (Result Http.Error ())
    | ToggleVersion VersionToggleAction Int
    | VersionToggled VersionToggleAction Int (Result Http.Error ())
    | PinIconHover Bool
    | Hover Hoverable
